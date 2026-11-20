# 本地 vs 上游 QuantumNous/new-api 功能差异报告

- **报告时间**：2026-11-20
- **本地 HEAD**：`5f1541d`（`zakolinxi/new-api-main-copy`）
- **上游 HEAD**：`48f9df9`（`QuantumNous/new-api` main，2026-11-20 浅取快照）
- **性质**：纯对比，未改动任何代码
- **方法**：两侧**无共同祖先**（本地首个提交 `c44291e` 是 clean import），无法按 commit 历史对齐。
  因此采用三路交叉验证：① 文件树差集（本地独有 169 个 `.go` 文件 / 上游独有 71 个）；
  ② 端点集合差集（本地 280 / 上游 242）；③ 关键符号全库检索（`git grep` 判定上游是否存在同名实现）。

---

## 一、总体量化

| 维度 | 本地 | 上游 | 差异 |
|---|---|---|---|
| 渠道类型常量 | 63 | 59 | 本地 = 上游 59 + **4 个独有**，无缺失 |
| API 端点 | 280 | 242 | 本地多 38 |
| Go 源文件 | 3035 | 2840 | 本地多 195 |
| 前端架构 | **双前端**：`web/default`(React19) + `web/classic`(Semi Design) | **单前端** `web/src` | 路线分歧 |
| 任务渠道（视频/音乐） | **10 个原生适配器** | 仅 `taskcommon` + `jsplugin` | 架构分歧 |

**渠道类型差集（精确）**

- 本地独有 4：`ChannelTypeVercel`、`ChannelTypeCodeBuddy`、`ChannelTypeClaudeCode`、`ChannelTypeCodexCompatibility`
- 上游独有：**0 个**（上游全部 59 种本地都有）

---

## 二、渠道与转发能力

| # | 功能 | 说明 | 关键实现 | 上游 |
|---|---|---|---|---|
| 1 | **Vercel AI Gateway 渠道** | 完整渠道实现：请求/响应/流式转换、模型列表 | `relay/channel/vercel/{adaptor,constants,convert,dto,response}.go`、`constant/api_type.go` | 无 |
| 2 | **CodeBuddy 渠道** | 客户端身份头伪装、profile 管理、系统提示注入、会话清理 | `relay/channel/codebuddy{,_profile,_cleanup,_system_prompt}.go` | 无 |
| 3 | **Claude Code 渠道** | Claude Code 客户端专用通道 | `ChannelTypeClaudeCode` | 无 |
| 4 | **Codex 兼容模式** | 把普通 OpenAI 渠道伪装成 Codex 上游（客户端身份兼容层） | `relay/channel/compatibility.go`、`relay/channel/openai/compatibility.go` | 无 |
| 5 | **视频/音乐任务原生适配器 ×10** | 阿里、豆包、Gemini、海螺、即梦、可灵、Sora、Suno、Vertex、Vidu，各含轮询/取结果/计费 | `relay/channel/task/{ali,doubao,gemini,hailuo,jimeng,kling,sora,suno,vertex,vidu}/` | 已全部插件化（`task/jsplugin`），**仅剩 taskcommon** |
| 6 | **渠道并发限制** | 限制同渠道在途请求数，超出即拒；模型级配置独立计数且不受渠道级管辖；`ChannelConcurrencyEnabled/Default` 两个全局项 | `common/channel-concurrency.go`、`service/channel_concurrency.go`、`setting/channel_concurrency.go` | 无 |
| 7 | **模型健康度** | 从日志表聚合：总览（监控模型数/健康数/成功率/优秀模型）、按小时时间线、按模型排名，前端独立 ModelHealth 页 | `model/model_health.go`、`controller/model_health.go`、`web/classic/src/pages/ModelHealth` | 无 |
| 8 | **模型可用性视图** | 按渠道×模型映射统计可用性，含 `/api/model-availability` 与前端 `model-availability` 模块 | `controller/model_availability.go` | 无 |
| 9 | **渠道测试探针** | 用真实的 `channel_test_echo` 工具调用探测渠道能否正确执行 function calling，而非只看 HTTP 200 | `controller/channel_test_probe.go`、`channel_test_diagnostics.go` | 仅有基础连通性测试，无工具调用探针 |
| 10 | **客户端身份版本追踪** | 按渠道记录 OpenAI SDK 身份版本，便于发现客户端漂移 | `controller/client_identity_versions.go`、`service/client_identity_versions.go` | 无 |
| 11 | **上游响应拦截** | 对上游返回内容做 keyword/regex 规则匹配，命中后 `remove`（剔除）或 `block`（阻断），配置存 `UpstreamInterceptionConfig` | `setting/upstream_interception.go`、`service/upstream_interception_writer.go` | 无 |
| 12 | **请求体调试记录** | 把进站请求体按 gzip 分块落库，可在日志页按 `request_id` 回看原始 body；含敏感字段脱敏与磁盘/内存双模式 | `model/request_debug_body.go`、`common/request_debug.go` | 无 |
| 13 | **渠道队列预热** | 按渠道 `Queue` 配置预热上游队列（并发/超时/重试可调），避免冷启动抖动 | `controller/channel_queue_warmer.go` | 无 |
| 14 | **渠道自定义余额** | 手工/自定义渠道余额维护，与「从上游计费 API 刷新」并存 | `model/channel_custom_balance.go`、`service/channel_custom_balance.go` | 有 `channel_balance`（仅 API 拉取），**实现不同** |
| 15 | **内部基础设施** | 分布式命名租约、渠道请求守卫、渠道变更封装、预扣额度独立模块 | `model|service/named_lease.go`、`service/request_guard.go`、`service/channel_mutation.go`、`service/pre_consume_quota.go` | 无同名实现 |
| 16 | **游乐场联网搜索** | Playground 内置 Bing 搜索工具，模型可实时检索 | `controller/playground_web_search.go`、`pkg/bingsearch/` | 无（上游的 web_search 是转发层给模型用的，不含 Playground 侧） |

---

## 三、计费与定价

| # | 功能 | 说明 | 关键实现 | 上游 |
|---|---|---|---|---|
| 17 | **记账余额 `charged_quota`** | 把「充值进来的钱」与「额度 quota」拆成两个账本，独立记账与校验；连带改造额度调整、改名扣费、用户间转账、资金来源（钱包/订阅） | `model/user.go`、`model/user_quota_adjustment.go`、`model/user_transfer.go`、`service/funding_source.go` | 仅订阅模型里出现该字段，**无用户级独立账本** |
| 18 | **套餐付款方式限制** | 订阅/套餐可限制只允许指定支付渠道（Stripe / Creem / ePay / Waffo Pancake），下单与前端同时校验 | `controller/subscription*.go`、`web/classic/.../AddEditSubscriptionModal.jsx` | 无 |
| 19 | **价格补丁服务 `pricing/patch`** | 定价改动走独立补丁接口：`set`（CAS 校验）/ `set_if_missing` / `delete`，`PUT /api/option/` 对 pricing 类 key 直接拒绝 | `controller/pricing_options.go`、`service/pricing_options.go`、`model/pricing_options.go` | 无（上游的 `model_pricing_config.go` 是模型定价配置，语义不同） |
| 20 | **自动价格/模型同步** | 渠道增删改自动入队同步事件，`auto_price_guard` 状态机守卫价格不被覆盖，快照对比后只补缺失 | `service/auto_sync_*.go`、`model/auto_price_guard.go`、`model/auto_sync_event.go`、`service/channel_auto_sync_snapshot.go`、`controller/auto_sync*.go` | 无 |
| 21 | **上游定价归一化** | 从标准源 / OpenRouter / models.dev 三路拉取，归一化后生成缺失定价补丁 | `service/upstream_pricing.go`（`FetchNormalizedPricing`、`BuildMissingPricingPatch`） | 有 `ratio_sync`（同步倍率），**覆盖范围不同** |
| 22 | **模型元数据同步** | 从 modelmap.dev 同步模型与供应商元数据，带本地 catalog 缓存 | `service/model_metadata_sync.go`、`service/model_metadata_catalog.go` | 无 |
| 23 | **价格设定精度修复** | 修复定价小数精度丢失 | `d96351d` | — |

---

## 四、用户与安全治理

| # | 功能 | 说明 | 关键实现 | 上游 |
|---|---|---|---|---|
| 24 | **用户级自动封禁** | 4 类规则：敏感词、UA、**超额 IP**、**模型探测**；`observe`/`enforce` 双模式；带封禁记录查询/释放、用户列表徽章、封禁理由字段 | `setting/auto_ban.go`、`service/auto_ban.go`、`model/user_auto_ban.go`、`middleware/relay_auto_ban.go`、`controller/auto_ban.go` | 只有**渠道级** `auto_ban`，无用户级 |
| 25 | **IP 黑名单** | 精确 IP + CIDR 规则、规范化、快照匹配、按登录 IP 封用户、被封返回样式化 403 页 | `model/ip_ban.go`、`middleware/ip_ban.go`、`controller/ip_ban.go`、`web/classic/src/pages/IpBan` | 无 |
| 26 | **浏览器指纹封禁** | 指纹黑名单 CRUD 与请求匹配 | `model/browser_fingerprint_ban.go`、`controller/browser_fingerprint_ban.go`、`web/classic/.../BrowserFingerprintBan` | 无 |
| 27 | **转发 UA 黑名单** | 按 UA 匹配拒绝转发请求，可配置动作 | `common/relay_user_agent_blacklist.go`、`middleware/relay_user_agent_blacklist.go` | 无 |
| 28 | **超级管理员转让** | 当前超管可把 root 身份转给他人，转让后原超管降级为指定角色（默认普通用户）；同时放开「同级管理员可操作自己」 | `controller/user.go`（转让分支） | 无 |
| 29 | **用户自助改名** | 事务内改名并扣费，防止并发重复扣 | `model/user_rename.go`、`controller/user_rename.go` | 无 |
| 30 | **用户间额度转账** | 用户 A 转额度给用户 B，收 3% 手续费，事务保证 | `model/user_transfer.go`、`controller/user_transfer.go`、`/api/user/transfer` | 无（上游只有 aff 转额度） |
| 31 | **邮箱测试发送** | 用户侧触发测试邮件验证 SMTP 配置 | `controller/user_email.go` | 无 |
| 32 | **注册完成流程** | OAuth/邀请注册后的补全步骤（`/register/complete`） | `controller/registration_completion.go` | 无 |
| 33 | **管理员权限配置** | 在权限目录之上提供配置入口：一次设置用户的角色、侧边栏模块可见性、权限映射 | `controller/permission_admin.go`（`GetPermissionAdmin` / `UpdatePermissionAdmin`） | 上游有 `/authz` 目录与 casbin 执行器，**无此配置入口** |
| 34 | **登录代理** | 登录相关外部请求统一走可配置代理，URL 归一化 | `setting/login_proxy.go`、`service/login_http_client.go` | 无 |
| 35 | **按用户请求限流** | 每用户每分钟请求数上限（Redis/内存双实现） | `middleware/user-request-rate-limit.go` | 无 |
| 36 | **高危漏洞修复** | `New-Api-User` 认证绕过等一批安全问题（14 文件 / 891 行） | `a20048a` | — |
| 37 | **签到安全边界** | 签到奖励按「钱包额度域」判定，防止跨域刷额度 | `9a18e7c` | — |

---

## 五、运营与内容

| # | 功能 | 说明 | 上游 |
|---|---|---|---|
| 38 | **工单中心** | 状态机 `processing/waiting/resolved/closed`（closed 为终态）、用户与管理员双侧未读追踪、按关键词/状态/用户名筛选、用户面板 + 管理面板 + 详情抽屉 | 无 |
| 39 | **横幅 Banner** | CRUD + 公共 `/banners` 接口 + classic 首页展示 + 旧公告配置迁移 | 无 |
| 40 | **用户头像上传** | 上传校验（类型/尺寸）、本地/对象存储、OAuth 头像拉取、classic 主题支持 | 无 |
| 41 | **邀请码** | 注册用邀请码（`enabled/disabled/used` 状态机、批量生成、事务消耗、用户侧校验） | 无（上游只有兑换码） |
| 42 | **游戏中心** | 7 种玩法（德州/轮盘/贪吃蛇/1024/黄金矿工/Token 挖矿/股票期货）+ 事件驱动股市引擎（新闻/宏观事件模板、订单簿、K 线）+ 积分兑额度（普通 1000 分 = $1；肉鸽 NEON-PULSE 100000 分 = $1） | 无 |
| 43 | **自定义品牌图标** | 注册 Dots Studio / Aedilic / MiniCPM / Poolside / Other 图标，两套前端同步 | 无 |
| 44 | **系统设置搜索** | 设置项全文搜索，跨分组跳转 | 无 |
| 45 | **上游账号管理** | 上游站点账号 CRUD（加密凭据）、自动签到、余额刷新、健康检查、操作日志，前端 `UpstreamAccount` 页 | 无 |
| 46 | **霓虹脉冲兑换比例** | 游戏币兑换比例前端展示 | 无 |

---

## 六、日志与可观测

| # | 功能 | 说明 | 上游 |
|---|---|---|---|
| 47 | **日志折叠视图** | 按「用户×模型」折叠消费日志：组内计数 > 阈值(10) 才折叠，窗口 1000（上限 5000）；聚合 token/cache/quota，IP/时延取最新；两套前端各有折叠弹窗；ClickHouse 无主键时折叠后再分配展示 id | 无（`model/log_collapse.go`） |
| 48 | **强制记录真实客户端 IP** | 从 `CF-Connecting-IP` / `X-Forwarded-For` 等头还原真实客户端 IP 并落日志 | 无 |
| 49 | **请求体调试** | 见 #12 | 无 |

---

## 七、前端

| # | 差异 | 说明 |
|---|---|---|
| 50 | **双前端并存** | `web/default`（React 19 + Tailwind v4 + TanStack，25 个 feature 模块）+ `web/classic`（Semi Design，35 个页面，含游戏/工单/IP 封禁/指纹封禁/上游账号等本地独有页面）。上游已收敛为单前端 `web/src`。 |
| 51 | **Bearer + refresh-token 会话流** | default 端改用 Bearer token + 刷新令牌，替代 cookie 会话 |
| 52 | **登录态恢复** | `bootstrapAuthentication`：整页刷新后自动恢复会话，不再被踢回登录页 |
| 53 | **classic 仪表盘能力移植** | 把上游新版仪表盘能力反向移植进 classic |

---

## 八、工程与运维

| # | 差异 | 说明 |
|---|---|---|
| 54 | **文档体系重写** | README 全家桶重写、`docs/file-map.md` 逐文件清单（676 行）、渠道/安装补充文档、生产标识脱敏 |
| 55 | **Dockerfile 可构建性修复** | 补 `COPY relaykit ./relaykit`（`go.mod` 有本地 replace，缺这行干净 clone 构建必挂） |
| 56 | **停止跟踪运行时文件** | SQLite WAL/SHM、运行日志移出版本控制 |
| 57 | **空分组修复** | 分组为空时的保存与展示问题 |

---

## 九、看起来像新增、其实上游也有（勿误判）

签到（`controller/checkin.go`）、敏感词（上游已改为 instance 化 + 迁移）、`reasonmap`、
`relayconvert`/`openaicompat`（本地的 chat↔responses 互转与上游 relaykit 等价）、`ratio_sync`、
casbin `authz` 执行器与 `/authz` 权限目录（`GetPermissionCatalog` 两侧都有，本地多的只是配置入口）、
兑换码、订阅与多支付网关、Passkey/2FA、渠道亲和性、模型元数据 CRUD、
`perf_metrics`、`system_task`、Webhook、SSRF 防护、渠道余额（实现不同）、渠道级 `auto_ban`。

---

## 十、上游同期新增、本地尚未跟进（风险提示）

| 方向 | 上游新增 |
|---|---|
| **渠道插件化** | `pkg/jsplugin/`（`provider.go`/`transform.go`/`media.go`/`config_schema.go`…）、`service/provider_plugin*.go`、`controller/provider_plugin.go`、`relay/channel/providerplugin/` |
| **请求插件化** | `relay/requestplugin/`（附件、PDF、媒体、参数覆盖、计划、记录）、`setting/request_plugin_setting/` |
| **访问令牌作用域** | `service/access_token_scope.go`、`model/user_access_token.go`、`middleware/access_token_routes.go` |
| **多节点同步** | `service/node_sync.go`、`model/system_sync_task.go` |
| **日志导出** | `service/log_export.go`、`setting/operation_setting/log_setting.go` |
| **各家 web_search** | `relay/channel/{openai,deepseek,ali,xai,zhipu_4v}/web_search.go`、`relay/deepseek_tools_handler.go` |
| **渠道优先级** | `controller/channel_priority.go`、`model/channel_priority.go` |
| **请求补丁** | `pkg/requestpatch/patch.go` |
| **Token 工具包** | `tokenkit/{tokenkit,tokenizer,image}.go` |
| **其他** | `middleware/frontend_static.go`、`model/auth_cache_integrity.go`、`common/{email_resend,log_text,message,container_usage}.go`、`relay/channel/aws/mantle.go`、`relay/channel/ali/embedding.go`、`relaykit/relayconvert/convmeta/websearch.go` |

> 结论：本地在**运营侧（工单/游戏/横幅/邀请码/头像）、治理侧（用户封禁/IP/指纹/UA）、渠道扩展（Vercel/CodeBuddy/ClaudeCode）与任务渠道原生适配**上明显领先；
> 上游在**插件化架构（JS 渠道插件、请求插件）、多节点协同、web_search 生态**上领先。
> 两侧在「任务渠道」上已是**两套架构**，后续合并成本会持续上升。
