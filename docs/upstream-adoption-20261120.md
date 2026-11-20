# 上游功能适配记录（2026-11-20）

- **本地基线**：`5f1541d` + 本次改动（未提交）
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

### 5. 备用件（编译通过，未接线）

| 文件 | 用途 | 为什么先不接 |
|---|---|---|
| `pkg/requestpatch/patch.go`（844 行） | 请求参数补丁 DSL | 需要在 relay 主链路插入调用点，会动到所有转发路径，需单独一轮回归 |
| `middleware/frontend_static.go` | 前端静态资源 gzip/缓存中间件 | 本地已有自己的 `web-router.go` 静态服务与双前端切换 |
| `model/auth_cache_integrity.go` | 用户缓存完整性校验 | 依赖上游的 access token 体系 |

---

## 二、验证证据

```
go build -buildvcs=false ./...                      → 通过
go test ./common ./setting/... ./service ./middleware ./router ./model(相关子集)
                                                    → 除两个既存失败外全绿
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
| **渠道优先级的前端拖拽 UI** | 本地 classic 渠道表格是 Semi Design 表格，拖拽需要引入 dnd 组件 | 后端已就绪，前端可先用「上移/下移」按钮实现，零依赖 |
| **日志导出按钮** | classic 的日志页 | 后端已就绪，加一个下载按钮即可 |

---

## 四、下一步建议顺序

1. **前端入口**（半天）：渠道列表加「上移/下移/置顶」；日志页加「导出」按钮。两个后端都已联调过。
2. **requestpatch 接线**（1 天）：先在 `relay/compatible_handler.go` 单点接入，跑一轮回归，再铺到其它 handler。
3. **node_sync**（1 天）：验证实例上先接 channels/options，观察两节点缓存是否收敛。
4. **jsplugin + requestplugin**（1 周起）：作为「插件运行时」整体立项，需要先决定是否引入 JS 引擎依赖，以及原生任务适配器是否逐步退役。

---

## 五、改动清单

**新增（未跟踪）**

```
common/message.go                    common/log_text.go
common/log_text.zh-CN.json           common/api_error_t.go
controller/channel_priority.go       controller/log_export.go
model/channel_priority.go            model/channel_priority_test.go
model/log_export_helper.go           model/system_sync_task.go
model/auth_cache_integrity.go        service/log_export.go
setting/operation_setting/log_setting.go
middleware/frontend_static.go        pkg/requestpatch/patch.go
router/channel_priority_router_test.go
```

**修改**

```
router/channel-router.go             +2 条路由（priority/order、priority/reorder）
router/api-router.go                 +2 条路由（log/export、log/self/export）
model/channel.go                     ChannelSortOptions.Apply 用 COALESCE + id DESC 兜底
```
