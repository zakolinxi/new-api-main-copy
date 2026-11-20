# 上游功能适配记录（2026-11-20）

- **本地基线**：`2dd0d80`（第一批：渠道优先级 + 日志导出 + 密钥前缀）+ 第二批（requestpatch 接线 + default 前端入口）
- **上游基线**：`QuantumNous/new-api@48f9df9`
- **原则**：只移植，不改本地既有行为；每个可执行单元先编译、再跑测试、最后跑真实实例验证

---

## 一、已完成并验证

### 1. 渠道优先级排序（已接线，可直接调用）

| 项 | 内容 |
|---|---|
| 端点 | `GET /api/channel/priority/order`、`POST /api/channel/priority/reorder` |
| 权限 | ChannelRead / ChannelWrite（与上游一致） |
| 实现 | `model/channel_priority.go`（247 行，上游原样）、`controller/channel_priority.go`（40 行） |
| 特性 | 乐观锁 revision 防并发覆盖；支持 preview 试算；undo/restore 回滚；同事务写 `abilities.priority` 保证选路与列表一致 |
| 测试 | `model/channel_priority_test.go`（209 行，上游原样）→ **通过** |

**顺带修掉一个真实缺陷**（上游的 bug fix，本地缺）：

`model/channel.go` 的 `ChannelSortOptions.Apply` 原先按裸 `priority` 列排序，导致
① `NULL` 的渠道排到错误位置（SQLite/MySQL/PG 对 NULL 排序方向不一致）；
② `priority` 相同的渠道顺序不稳定，翻页时行会跳动。
现改为 `COALESCE(priority, 0)` + `id DESC` 兜底，与选路时的「缺失即 0」语义对齐。

### 2. 日志导出（已接线）

| 项 | 内容 |
|---|---|
| 端点 | `GET /api/log/export`（管理员）、`GET /api/log/self/export`（用户，受开关控制） |
| 实现 | `controller/log_export.go`（自写，复用本地的 `model.GetAllLogs` / `GetUserLogs` 分页循环，避免与本地 `log.go` 的折叠逻辑冲突） |
| 上游组件 | `service/log_export.go`（Redis/内存双实现的导出互斥锁）、`setting/operation_setting/log_setting.go`（`user_export_enabled` / `max_range_days`） |
| 格式 | NDJSON，一行一条，末尾追加 `{"success":true,"data":{"count":N}}`；`X-Accel-Buffering: no` 让 nginx 不缓冲 |
| 安全 | 每用户同时只允许一个导出（跨节点靠 Redis 锁，1 分钟租约自动过期）；时间范围被 `max_range_days` 裁剪；单次上限 10 万行；沿用列表的字段脱敏（`FormatAdminLogs` / `FormatRootLogs` / `FormatUserLogsForExport`） |

### 3. 消息与日志的 i18n 基础件（被上面两项依赖）

`common/message.go`（`Message` + `NewMessage`）、`common/log_text.go` + `common/log_text.zh-CN.json`（93 KB 中文日志翻译表）、`common/api_error_t.go`（本项目自写的 `ApiErrorT`，在有前端翻译时带 `message_key` / `message_params`，无则退化为英文原文）。

### 4. 多节点同步的任务表（已就位，未接线）

`model/system_sync_task.go`（148 行）：7 个同步 topic（options/channels/authz/plugins/oauth_providers/models/subscription_plans）、`BumpSyncTopic` / `bumpSyncTopic` / `GetLatestSyncRevisions` / `FinishSyncTasks`。渠道优先级依赖它，所以一并移植；`node_sync` 服务本身尚未接线。

### 5. requestpatch 接线（已接线，按渠道开关）

| 项 | 内容 |
|---|---|
| 库 | `pkg/requestpatch/patch.go`（844 行，上游原样，含 237 行上游单测）|
| 配置 | 渠道 `setting` JSON 新增 `request_patch` 字段（`relaykit/dto.ChannelSettings.RequestPatch`，`json.RawMessage`，relaykit 只当不透明数据存）|
| 生效点 | `relay/channel/api_request.go` 的 `DoApiRequest`：读完 body → 建好请求头 → 应用 effects → 再让 Header Override 覆盖 |
| 语义 | `json.patch`（RFC 6902，含 add/remove/replace/move/copy/test）+ `header.patch`（set/remove）；`Apply` 是**全有或全无**，任一 op 被拒则整条请求不改 |
| 边界 | body 上限走 `MAX_REQUEST_BODY_MB`（默认 128MB）；`Authorization` / `Host` / `Content-Length` / `Content-Type` / hop-by-hop 头受保护，改不动 |
| 保存校验 | `model.Channel.ValidateSettings()` 在存库时就解码一遍，非法 effects 直接拒绝，不会拖到线上每个请求才炸 |
| 测试 | `relay/channel/request_patch_test.go`（7 个用例）+ `model/channel_settings_test.go::TestChannelValidateSettingsChecksRequestPatch`（4 个子用例）→ **全部通过** |

**与上游的差别（有意）**：上游的 effects 来自 JS 渠道插件的 `prepareRequest` 钩子，那套宿主本地没有；
这里把生产者换成**渠道级配置**，于是同一个 `requestpatch` 库在本地就有了实际用途，且不引入 JS 引擎。
本地的 `param_override` / `header_override` 仍走原来那条路，两者互不干扰 ——
`request_patch` 用于表达 `param_override` 的 gjson 模式写不出来的 RFC 6902 操作（数组下标增删、`test` 断言等）。

**不配置就等于不存在**：`channelRequestPatchEffects` 在 setting 为空时返回零 effects，
`DoApiRequest` 连 body 都不读，仍然把原始 reader 交给 net/http，热路径零额外开销。

### 6. 自定义 API 密钥前缀（本地独有）

| 项 | 内容 |
|---|---|
| 默认值 | `sk-`（历史形状，改配置前一切照旧）|
| 配置 | 选项 `TokenKeyPrefix`，示例 `sk-mao` → 归一化成 `sk-mao-`；上限 16 字符，只允许字母/数字/`-`/`_` |
| 存储 | `setting/token_key_setting.go`，`atomic.Value` 存前缀（每个转发请求都要读，不能上锁）|
| 选项三件套 | `InitOptionMap`（默认值）/ `normalizeOptionValue`（归一化）/ `updateOptionMap`（落运行时），缺一处就会重启丢失或保存不生效 |
| 兼容 | `TrimTokenKeyPrefix` 匹配不到配置前缀时**回落到 `sk-`**，所以改前缀后已签发的旧密钥继续可用 |
| 下发 | `/api/status` 增加 `token_key_prefix`，前端拼接不用硬编码 |
| 后端改造点 | `middleware/auth.go`（3 处）、`controller/token.go`、`model/token.go`、`service/quota.go` —— 全部改走 `TrimTokenKeyPrefix` |
| 前端改造点 | classic：令牌列表 / CCSwitch / Chat 链接；default：`lib/token-key.ts` + chat-links + cc-switch-dialog |
| 测试 | `setting/token_key_setting_test.go`（归一化、旧密钥兼容、示例形状）→ **通过** |

### 7. 前端入口（两套前端都已补）

| 端 | 入口 |
|---|---|
| classic | 渠道页「更多 → 优先级排序」弹窗（上移/下移，带 revision，冲突自动重拉）；使用日志「导出日志」按钮；系统设置 → 其他「API 密钥前缀」 |
| default | 渠道页「更多 → Channel Priority Order」对话框；通用日志工具栏下载图标；系统设置 → 站点 → 系统信息「API Key Prefix」 |

default 的导出按钮直接用当前 URL 里的筛选条件（时间范围/模型/分组/类型/请求 ID），
导出失败时后端返回的是 JSON，前端按 `blob.type` 识别后弹 toast，而不是把错误页存成 `.ndjson`。

### 8. 多节点同步的任务表（已就位，未接线）

`model/system_sync_task.go`（148 行）：7 个同步 topic（options/channels/authz/plugins/oauth_providers/models/subscription_plans）、`BumpSyncTopic` / `bumpSyncTopic` / `GetLatestSyncRevisions` / `FinishSyncTasks`。渠道优先级依赖它，所以一并移植；`node_sync` 服务本身尚未接线。

### 9. 备用件（编译通过，未接线）

| 文件 | 用途 | 为什么先不接 |
|---|---|---|
| `middleware/frontend_static.go` | 前端静态资源 gzip/缓存中间件 | 本地已有自己的 `web-router.go` 静态服务与双前端切换 |
| `model/auth_cache_integrity.go` | 用户缓存完整性校验 | 依赖上游的 access token 体系 |

---

## 二、验证证据

```
go build -buildvcs=false ./...                      → 通过
go vet ./relay/channel ./model ./setting            → 通过
go test ./common ./setting/... ./service ./middleware ./router ./relay/channel ./relay/common
                                                    → 除两个既存失败外全绿
go test -run RequestPatch ./relay/channel/          → 7 个用例通过
go test -run TestChannelValidateSettings ./model/   → 通过（含 request_patch 校验 4 个子用例）
web/default: tsgo -b                                → 本次改动的文件无类型错误（auth/lobe-icon 的报错是既存）
web/default: rsbuild build                          → 通过（Total 60543.8 kB）
web/classic: rsbuild build                          → 通过（Total 23024.0 kB）
真实实例（--port 3012）：
  /api/status                       → 200
  /api/channel/priority/order       → 401（已注册，需鉴权）
  /api/log/export?type=0            → 401（已注册，需鉴权）
  /api/log/self/export?type=0       → 401（已注册，需鉴权）
  /api/channel/priority/nonexistent → 404（对照组，证明不是通配）
```

**既存失败（在干净 `5f1541d` 上同样失败，与本次改动无关）**：

- `router: TestWebFallbackDoesNotCacheMissingAPIOrAssets`（需要 `web/*/dist` 产物）
- `pkg/perf_metrics: TestSummaryKeepsLegacySamplesAlongsideHourlySeriesAndFiltersGroups`

---

## 三、有意没做的，以及原因

| 上游功能 | 阻塞点 | 建议做法 |
|---|---|---|
| **渠道 JS 插件化**（`pkg/jsplugin` + `service/provider_plugin*` + `relay/channel/providerplugin`） | 需要引入 JS 引擎（上游用 `moego`/`moejs`/`sobek`，本地 go.mod 没有）；且与本地 10 个原生任务适配器的架构**互斥**，并存会出现两套渠道执行路径 | 单独立项：先只接「读」路径（用插件描述渠道元数据），转发仍走原生 adaptor |
| **请求插件化**（`relay/requestplugin`：附件/PDF/媒体/参数覆盖） | 要改 `relay/*_handler.go` 主链路，需新增 `ledongthuc/pdf` 依赖；影响所有转发端点 | 与 jsplugin 一起做，作为「插件运行时」第二阶段 |
| **访问令牌作用域**（`service/access_token_scope.go`、`model/user_access_token.go`、`middleware/access_token_routes.go`） | 上游是**多令牌表**（name/scopes/过期），本地是**单令牌字段**（`users.access_token`）。直接替换会让 classic 前端的令牌页失效，且 `UpdateUserAccessToken` / `GetAuditLogs` 同名冲突 | 先在 `users` 表加 `scopes` 字段做「单令牌 + 作用域」的轻量版；多令牌等前端重做 |
| **多节点同步**（`service/node_sync.go`） | 表已就位，但需要给每个 topic 写 reload 钩子并注册定时任务；本地生产是「生产 + cn 节点共库」，缓存一致性反而更敏感，需要窗口期 | 先在验证实例（3010/`new_api_v2`）跑通 channels/options 两个 topic |
| **各家 web_search**（openai/deepseek/ali/xai/zhipu_4v） | 要改 5 个 adaptor 的请求构造，且本地有 `relay/channel/openai/compatibility.go` 等自有关闭开关 | 按厂商逐个上，每次只改一个 adaptor |

（原表里「渠道优先级前端 UI」「日志导出按钮」两项已完成：两套前端都补了入口，
统一用上移/下移而不是拖拽，省掉一个 dnd 依赖。）

---

## 四、下一步建议顺序

1. **node_sync**（1 天）：验证实例上先接 channels/options，观察两节点缓存是否收敛。
2. **`request_patch` 前端编辑器**（半天）：现在只能在渠道 `setting` JSON 里手写，可以做成
   「参数补丁」旁边的可视化编辑器（复用 `param_override` 的编辑弹窗骨架）。
3. **jsplugin + requestplugin**（1 周起）：作为「插件运行时」整体立项，需要先决定是否引入 JS 引擎依赖，
   以及原生任务适配器是否逐步退役。requestpatch 已就位，届时只需把生产者从「渠道配置」换成「插件钩子」。

---

## 五、改动清单

**新增**

```
common/message.go                    common/log_text.go
common/log_text.zh-CN.json           common/api_error_t.go
controller/channel_priority.go       controller/log_export.go
model/channel_priority.go            model/channel_priority_test.go
model/log_export_helper.go           model/system_sync_task.go
model/auth_cache_integrity.go        service/log_export.go
setting/operation_setting/log_setting.go   setting/token_key_setting.go
setting/token_key_setting_test.go    middleware/frontend_static.go
pkg/requestpatch/patch.go            pkg/requestpatch/patch_test.go
relay/channel/request_patch.go       relay/channel/request_patch_test.go
router/channel_priority_router_test.go
```

**新增（前端）**

```
web/classic/src/components/table/channels/modals/ChannelPriorityModal.jsx
web/default/src/lib/token-key.ts
web/default/src/features/usage-logs/components/export-logs-button.tsx
web/default/src/features/channels/components/dialogs/channel-priority-dialog.tsx
```

**修改**

```
router/channel-router.go             +2 条路由（priority/order、priority/reorder）
router/api-router.go                 +2 条路由（log/export、log/self/export）
model/channel.go                     ChannelSortOptions.Apply 用 COALESCE + id DESC 兜底
                                     ValidateSettings 增加 request_patch 解码校验
model/option.go                      TokenKeyPrefix 三件套 + 默认值注册
middleware/auth.go                   trim 改走 setting.TrimTokenKeyPrefix（3 处）
controller/token.go                 同上
model/token.go                      同上
service/quota.go                    同上
controller/misc.go                  /api/status 下发 token_key_prefix
relay/channel/api_request.go        DoApiRequest 接入 request_patch（唯一生效点）
relaykit/dto/channel_settings.go    ChannelSettings 增加 RequestPatch
relaykit/types/error.go             增加 channel:request_patch_invalid
web/classic/src ...                 令牌前缀拼接 3 处、设置页、日志导出、渠道排序入口
web/default/src ...                 令牌前缀拼接 2 处、日志导出、渠道排序、系统信息设置
```
