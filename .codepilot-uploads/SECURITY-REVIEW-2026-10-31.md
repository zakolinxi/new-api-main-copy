# 安全审查报告：new-api 全量代码审查与修复

**审查范围**：全项目 3014 个文件（Go 后端 + `web/default` + `web/classic` 双前端主题）
**审查时间**：2026-10-31
**状态**：5 项已修复并附回归测试，1 项仅加固未完全消除

---

## 一、修复清单

| # | 严重级别 | 问题 | 位置 | 状态 |
|---|---|---|---|---|
| 1 | **严重** | `New-Api-User` 头可冒充任意用户（含 root） | `middleware/auth.go` | 已修复 |
| 2 | 高 | CORS 通配符与凭据许可自相矛盾 | `middleware/cors.go` | 已修复 |
| 3 | 中 | `DeleteSelf` 重复调用删除逻辑 | `controller/user.go` | 已修复 |
| 4 | 中 | `SaveTmpFile` 失败时泄漏临时文件 | `common/utils.go` | 已修复 |
| 5 | 中 | `checkin` 月份参数未校验 | `model/checkin.go` | 已修复 |
| 6 | 低 | 2FA 敏感端点缺速率限制 | `router/api-router.go` | 已修复 |
| 7 | 低 | 渠道空分组未兜底（历史遗留） | `model/channel.go` | 代码已修复，历史数据需手工迁移 |

---

## 二、漏洞 1（严重）：`New-Api-User` 认证绕过

### 2.1 原实现

```go
// 修复前 middleware/auth.go
func NewApiUserAuth() func(c *gin.Context) {
    return func(c *gin.Context) {
        apiUserIdStr := c.Request.Header.Get("New-Api-User")   // ← 唯一的"凭据"
        apiUserId, err := strconv.Atoi(apiUserIdStr)
        user, err := model.GetUserById(apiUserId, false)
        c.Set("id", user.Id)
        c.Set("role", user.Role)                              // ← 直接采用目标用户角色
        c.Next()
    }
}
```

该中间件完全不校验调用方身份，只凭一个自定义请求头就把**目标用户**的 `id` 和 `role` 写入 Gin 上下文。这等同于把请求头当作认证凭据。

### 2.2 可利用性验证

修复前实测（仅发送 `New-Api-User: 1`，无任何 `Authorization`）：

```
status=200 body={"seen_id":1,"seen_role":100}
```

`role=100` 即 `RoleRootUser`。**无需任何凭据即可获得 root 身份**。

### 2.3 影响面

挂载点（修复前）：

- `router/api-router.go:108` — `selfRoute` 整个分组，含 `PUT /self`、`DELETE /self`、`POST /transfer`、`POST /token`、`POST /aff_transfer`、`POST /rename`、passkey、2FA 等全部用户自助接口
- `router/api-router.go:188` — `checkinRoute`，含 `GET/POST /checkin`

典型攻击路径：

| 接口 | 后果 |
|---|---|
| `POST /api/user/transfer` | 把任意用户余额转入攻击者账户 |
| `POST /api/user/token` | 为任意用户签发管理 AccessToken（持久化后门） |
| `DELETE /api/user/self` | 删除任意账号（含 root） |
| `POST /api/user/rename` | 重命名任意账号 |

### 2.4 旁证：缺失的比对逻辑

`i18n/keys.go:40` 定义了 `MsgAuthUserIdMismatch`，三个语言文件也都有对应文案：

```yaml
# zh-CN.yaml:34
auth.user_id_mismatch: "无权进行此操作，New-Api-User 与登录用户不匹配"
```

但修复前**全代码库零引用**——说明本应存在的"请求头与已认证用户是否一致"比对被遗漏了。

### 2.5 修复方案

`NewApiUserAuth` 改为先走真实凭据校验（`authenticateDashboardRequest`，支持 dashboard access token 与 PAT），再把 `New-Api-User` 降级为一致性校验：

```go
func NewApiUserAuth() func(c *gin.Context) {
    return func(c *gin.Context) {
        user, identity, useAccessToken, err := authenticateDashboardRequest(c)
        if err != nil {
            writeDashboardAuthError(c, err)
            return
        }
        // ... 状态、封禁、用户名合法性检查 ...
        setDashboardAuthContext(c, user, identity, useAccessToken)
        if !requireNewApiUserHeaderMatch(c) {   // 新增：header 必须与已认证用户一致
            return
        }
        c.Next()
    }
}
```

`requireNewApiUserHeaderMatch` 行为：

- header 未发送 → 放行（兼容不发送该头的老客户端）
- header 格式非法 → `401 AUTH_USER_ID_FORMAT_ERROR`
- header 与已认证用户不一致 → `403 AUTH_USER_ID_MISMATCH`（复用已有 i18n 文案）

`UserOrNewApiUserAuth` 保留为 `NewApiUserAuth` 的兼容别名，不再存在"无凭据分支"。

### 2.6 兼容性确认

两个前端在**所有**请求上都会同时携带 `Authorization` 和 `New-Api-User`：

- `web/default/src/lib/api.ts:187-193`（请求拦截器）
- `web/default/src/lib/auth-session.ts:398`
- `web/classic/src/helpers/api.js:122`（401 重试时重新设置两个头）

因此正常用户流程不受影响。

### 2.7 回归测试

`middleware/auth_test.go` 新增 3 条：

| 测试 | 断言 |
|---|---|
| `TestNewApiUserHeaderCannotAuthenticateAlone` | 仅带 header、不带凭据 → `401`，且响应体不含 `"role":100` |
| `TestNewApiUserHeaderMismatchIsRejected` | 用 A 的 token 配 B 的 header → `403 AUTH_USER_ID_MISMATCH` |
| `TestNewApiUserHeaderMatchingAuthenticatedUserIsAllowed` | token 与 header 一致 → `200` 且 ID 正确 |

---

## 三、漏洞 2（高）：CORS 配置自相矛盾

### 3.1 原实现

```go
// 修复前 middleware/cors.go
config := cors.DefaultConfig()
config.AllowAllOrigins = true
config.AllowCredentials = true     // ← 与上一行互斥
```

`gin-contrib/cors@v1.7.2` 在 `AllowAllOrigins` 为真时下发 `Access-Control-Allow-Origin: *`，而浏览器明确拒绝 `*` 与凭据同时使用。所以这不是"任意站携带 cookie 读响应"的即时可利用漏洞，但配置在语义上试图开启跨域凭据，属于**危险的后备状态**：一旦有人把 `AllowAllOrigins` 换成反射 Origin 的 `AllowOriginFunc`，立刻变成真实的跨域凭据泄露。

### 3.2 修复方案

改为两种显式、互斥的模式：

| 模式 | 触发条件 | `Allow-Origin` | `Allow-Credentials` |
|---|---|---|---|
| 默认 | 未配置 `SESSION_COOKIE_TRUSTED_URL` | `*` | `false` |
| 可信代理 | 已配置可信来源 | 精确白名单 | `true` |

可信来源复用已有的 `common.SessionCookieTrustedURLs`（由 `InitSessionCookieSettings` 从 `SESSION_COOKIE_TRUSTED_URL` 解析并强制 https），并通过 `common.NormalizeOrigin` 做规范化比对，避免前缀/后缀绕过与大小写、默认端口差异。

### 3.3 回归测试

`middleware/cors_test.go` 新增 5 条，其中核心不变量：

```go
// 通配符来源绝不可与凭据许可同时出现
assert.Equal(t, "*", allowOrigin)
assert.NotEqual(t, "true", allowCredentials)
```

另外覆盖：默认模式不回显 Origin、可信模式仅放行白名单、拒绝 `console.example.com.evil.example` 与 `http://` 降级、接受 `:443` 等价写法。

---

## 四、漏洞 3（中）：`DeleteSelf` 重复删除

### 4.1 问题

`controller/user.go` 中 `DeleteUserForSession(identity)` 被连续调用两次，中间仅插入头像清理：

```go
if err := model.DeleteUserForSession(identity); err != nil { ... }
deleteUserAvatarObject(id, avatar)
if err := model.DeleteUserForSession(identity); err != nil { ... }   // ← 必然失败
```

第一次调用已吊销全部会话，第二次必然失败并走 `writeSecurityOperationError` 返回错误——**账号实际已删除，接口却返回失败**，前端误判为操作失败。

### 4.2 修复

删除第二段重复调用，`succeeded = true` 紧随头像清理之后。

---

## 五、漏洞 4（中）：`SaveTmpFile` 临时文件泄漏

### 5.1 问题

```go
f, err := os.CreateTemp(os.TempDir(), filename)
defer f.Close()
_, err = io.Copy(f, data)
if err != nil {
    return "", errors.Wrapf(...)   // ← 文件已创建，但未删除
}
```

上传中断或磁盘写满时，半成品文件永久残留在系统临时目录。

### 5.2 修复

显式关闭（并捕获 `Close` 错误，因为缓冲区数据可能在 `Close` 时才落盘失败），失败时删除文件：

```go
_, err = io.Copy(f, data)
if closeErr := f.Close(); err == nil && closeErr != nil {
    err = closeErr
}
if err != nil {
    if removeErr := os.Remove(f.Name()); removeErr != nil {
        SysError("failed to remove temporary file " + f.Name() + ": " + removeErr.Error())
    }
    return "", errors.Wrapf(...)
}
```

注意：`common/utils.go` 属于 `common` 包内部，调用 `SysError` 不能加 `common.` 前缀。

### 5.3 回归测试

`common/utils_tmpfile_test.go` 新增 2 条：失败后临时目录条目数不变；成功路径内容完整可读。

---

## 六、漏洞 5（中）：`checkin` 月份参数未校验

### 6.1 问题

```go
// 修复前 model/checkin.go
startDate := month + "-01"
endDate := month + "-31"       // ← 硬编码月末
```

`month` 直接来自 `c.DefaultQuery("month", ...)`，无任何格式校验。用 `-31` 作上界对短月份仅靠字符串序"恰好"正确，且任意输入都会进入查询。

### 6.2 修复

新增 `checkinMonthRange`，用 `time.Parse("2006-01", ...)` 校验格式，并由 `time.Date` 推导真实月末：

```go
start := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.UTC)
end := start.AddDate(0, 1, -1)   // 下月首日减一天 = 真实月末
```

### 6.3 回归测试

`model/checkin_month_test.go` 新增 3 组，覆盖 12 种非法输入（含 SQL 注入尝试字符串）、闰年 2 月（2024-02-29）、全部 12 个月的真实月末、首尾空白裁剪。

---

## 七、漏洞 6（低）：2FA 端点缺速率限制

`/2fa/disable` 与 `/2fa/backup_codes` 是唯一两个未挂 `UserCriticalRateLimit("security-verification")` 的安全敏感端点，与同组其他端点不一致。虽然 controller 内部调用 `RequireSecurityProof` 保证安全性，但缺少速率限制意味着 proof 可被无限次尝试消费。

已补齐，与 `setup`/`enable` 保持一致。

---

## 八、渠道空分组：根因已在代码层修复

原始 BUG 文档（`1790433391422-BUG-channel-group-empty.md`）记录"根因未修"。经核查**当前代码已修复**：

| 建议的修复方向 | 当前实现 |
|---|---|
| 空 `group` 兜底为 `default` | `model/channel.go:333-339` 的 `NormalizeGroup()`，在 `InsertTx`（:644）、`UpdateTx`（:665）、`service/channel_mutation.go:843` 三处调用 |
| `abilities` 按 `channel_id` 全删重建 | `model/ability.go:352` `UpdateAbilities` 按 `channel_id` 删除后重建，不依赖旧 group 值 |
| 回归测试 | `service/channel_mutation_test.go:125-130` 断言空分组被归一化并写入 abilities |

因此**新版本不会再产生空分组脏数据**，但存量脏数据仍需按原文档的手工 SQL 迁移（`abilities` 主键含 `group`，必须"插新删旧"）。

---

## 九、审查确认无问题的部分

| 检查项 | 结论 |
|---|---|
| SQL 注入 | 全库仅 1 处 `DB.Raw(fmt.Sprintf(...))`（`model/passkey_migration.go:138`），参数为内部常量 `indexName`；`Exec(fmt.Sprintf` 零命中 |
| 命令注入 | `exec.Command` 仅 `common/utils.go:34-38`（打开浏览器），参数为程序自身 URL |
| JWT 校验 | `service/auth_token.go:107-124` 的 `ParseUnverified` 仅用于类型分流，真正校验走 `ParseAccessToken`；算法固定 HS256，过期/iss/aud 均校验 |
| 密码存储 | bcrypt + argon2id 双支持；AES-GCM 带随机 nonce 与认证标签 |
| 支付幂等 | `model/topup.go:179`、`model/redemption.go:271` 均为"行锁 + 事务内状态机 + CAS `RowsAffected` 校验" |
| 余额转账 | `model/user_transfer.go:56` 按 ID 排序加锁避免死锁，`Where("quota >= ?")` 防负数 |
| SSRF | `service/protected_fetch_client.go:171` 在实际拨号前用解析后 IP 二次校验，杜绝 DNS rebinding |
| 路径穿越 | `service/avatar_storage.go:118-145` 双重校验（`path.Clean` + `filepath.Rel` 前缀检查） |
| 前端 XSS | `web/default/src/components/ui/markdown.tsx:699` 有 `DOMPurify.sanitize` |
| 兑换码随机性 | `controller/redemption.go:127` 使用 `uuid.New()`（加密安全） |

---

## 十、验证记录

| 命令 | 结果 |
|---|---|
| `go build ./middleware/` | 通过 |
| `go build ./common/` | 通过 |
| `go build ./model/` | 通过 |
| `go build ./router/` | 通过 |
| `go vet ./middleware/ ./common/ ./model/ ./router/` | 通过，无输出 |
| `go test ./middleware/ ./common/ ./model/` | `ok middleware 3.550s` / `ok common 6.401s` / `ok model 36.069s` |
| `go test ./service/` | `ok service 93.802s` |
| `go test ./controller/ -run "TestSecurityLoginFactorState..."` 等 | 通过 |
| `go test ./middleware/ -run "TestNewApiUserHeader"` | 3 条全通过 |
| `go test ./middleware/ -run "TestCORS"` | 5 条全通过 |
| `go test ./model/ -run "TestCheckinMonthRange"` | 3 组全通过 |
| `go test ./common/ -run "TestSaveTmpFile"` | 2 条全通过 |

**未完成的全量验证**：

- `go build ./...` 仍因 `web/classic/dist` 未构建而失败（`main.go:51` embed 指令），与本次改动无关
- `go test ./controller/` 全量执行超过 600 秒超时，改为按测试名分组验证

---

## 十一、遗留风险

1. **前端依赖未审计**：未执行 `bun install` / `npm audit`，`web/default` 与 `web/classic` 的第三方库漏洞未覆盖。
2. **`checkin` 接口返回原始错误**：`controller/checkin.go` 在 `model.GetUserCheckinStats` 失败时把 `err.Error()` 直接回给前端。当前 `checkinMonthRange` 的错误文案是固定的，不泄漏内部信息，但后续新增错误类型时需注意。
3. **`AllowHeaders: ["*"]` 保留**：请求侧头通配不影响响应侧来源策略，但如果后续引入需要严格约束的自定义头，应收紧为显式列表。
4. **上游适配器未逐一审查**：`relay/channel/*` 下 30+ provider 适配器仅抽样，未覆盖各家协议实现细节。
5. **`controller/` 大测试文件未通读**：`security_enrollment_test.go`（81KB）、`responses_websocket_test.go`（64KB）等未逐行审阅。

---

## 十二、上线建议

1. **漏洞 1 必须优先上线**，这是无需凭据即可获得 root 的入口。
2. 上线后立即检查历史审计日志，确认是否已有 `New-Api-User` 伪造痕迹：

   ```sql
   -- 检查是否有来自非常规来源的 self 分组写操作
   SELECT user_id, action, ip, created_at
   FROM audit_logs
   WHERE created_at > <上线前N天>
     AND category IN ('access_token', 'security')
   ORDER BY created_at DESC
   LIMIT 200;
   ```

3. 建议轮换所有用户的 `access_token`（`POST /api/user/token` 可被用来种持久化后门）。
4. 如生产环境通过反向代理暴露且前端与后端不同源，需显式设置 `SESSION_COOKIE_SECURE=true` 与 `SESSION_COOKIE_TRUSTED_URL`，否则 CORS 会停留在默认的"通配符无凭据"模式。
