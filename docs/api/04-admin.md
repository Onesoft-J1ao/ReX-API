# 管理 API（04）

本文件记录管理端接口：渠道、用户、模型与供应商、系统选项、日志与审计、兑换码、订阅、任务插件、系统任务、令牌分组、部署、自定义 OAuth、关联服务、性能与倍率同步、站内支持。所有路径以 `/api` 为前缀，注册于 `router/api-router.go`（下称 `api-router`）、`router/channel-router.go`（下称 `channel-router`）、`router/authz-router.go`、`router/task-router.go`、`router/video-router.go`。

## 通用约定

### 三个角色门槛

- `AdminAuth`（`middleware/auth.go:106`）：角色 ≥ `common.RoleAdminUser`。
- `RootAuth`（`middleware/auth.go:112`）：角色 = `common.RoleRootUser`。
- `RequirePermission(perm)`（`middleware/auth.go:235`）：在 `AdminAuth` 之上再校验 Casbin 权限。校验失败返回 **HTTP 403** + `{"success":false,"message":"<权限不足>"}`，**不含 `code` 字段**（与 `AdminAuth` 的失败响应不同）。判定逻辑见 `service/authz/resolver.go:8`：超级管理员角色（root）直接放行；否则先看用户级显式策略，再取角色基线并集。

### 权限资源与动作

定义在 `service/authz/`：

| 资源 | 动作常量 | 含义 | 默认角色 |
| --- | --- | --- | --- |
| `channel` | `read` | 查看渠道列表与详情（不含密钥） | admin（`service/authz/resources_channel.go:30`） |
| `channel` | `operate` | 测试渠道、刷新余额、启停单个/批量/按标签渠道 | admin（`service/authz/resources_channel.go:36`） |
| `channel` | `write` | 编辑非敏感设置（模型、分组、路由规则） | admin（`service/authz/resources_channel.go:42`） |
| `channel` | `sensitive_write` | 新建渠道或修改 key、base URL、覆盖项（**无默认角色**，`service/authz/resources_channel.go:44`） | 无 |
| `channel` | `secret_view` | 查看完整渠道密钥（保留位，`service/authz/resources_channel.go:50`） | 无 |
| `task_plugin` | `bind` | 列出已注册任务插件并在创建/编辑任务插件渠道时绑定（`service/authz/resources_task_plugin.go:15`） | 无 |
| `audit` | `read` | 查看其他账号的审计日志（root 记录始终排除，`service/authz/resources_audit.go:11`） | 无 |

root 角色是超级用户（`service/authz/role.go:24`，`Superuser: true`），因此**所有 `RequirePermission` 接口对 root 无条件放行**；admin 仅对带 `DefaultRoles: [admin]` 的动作默认放行。

### 管理写操作自动审计

`AdminAuth` 与 `RootAuth` 会在鉴权链路内自动为写操作留痕（`middleware/auth.go:73`、`middleware/auth.go:80`）。handler 内手动埋点（`recordManageAudit`）时会设置标记跳过兜底，避免重复（`middleware/auth.go:72`）。

### 响应与错误

与用户端一致：成功为 `{"success":true,"message":"","data":...}`（`common/gin.go:232`），业务失败多为 HTTP 200 + `{"success":false,"message":"..."}`。少数接口使用明确状态码，例如 `RequirePermission` 的 403、`ConfirmPaymentCompliance` 的 403（`controller/payment_compliance.go:31`）、`GetRatioConfig` 的 403（`controller/ratio_config.go:13`）、`TestAllChannels` 的 409（`controller/channel-test.go:1352`）、`SyncUpstreamModels` 的 400/409（`controller/model_sync.go:405`、`controller/model_sync.go:414`）、`UpdatePasskeyDomains` 的 409（`controller/option.go:268`）。

### 分页

同用户端：`p`、`page_size`（兼容 `ps`、`size`），上限 100（`common/page_info.go:11`、`common/page_info.go:65`）。

## 端点总表

### 渠道（channel）

路由声明于 `channel-router.go:39`（42 条 `channelPermissionRoutes`）与 `channel-router.go:23`（root-only）。所有接口在 `AdminAuth` 之下。

| 方法 | 路径 | 权限 | 用途 | 行号 |
| --- | --- | --- | --- | --- |
| GET | `/api/channel/` | `channel.read` | 分页渠道列表（`key` 被 `Omit`，见下） | `channel-router.go:40` |
| GET | `/api/channel/search` | `channel.read` | 按关键字/分组/模型/状态/类型搜索渠道 | `channel-router.go:41` |
| GET | `/api/channel/models` | `channel.read` | 渠道类型 → 模型名映射（`controller/model.go:354`） | `channel-router.go:42` |
| GET | `/api/channel/default_base_urls` | `channel.read` | 各渠道类型的默认 base URL | `channel-router.go:43` |
| GET | `/api/channel/models_enabled` | `channel.read` | 当前已启用的模型名 | `channel-router.go:44` |
| GET | `/api/channel/ops` | `channel.read` | 重试次数与自动禁用策略快照 | `channel-router.go:45` |
| GET | `/api/channel/:id` | `channel.read` | 单渠道详情（脱敏） | `channel-router.go:46` |
| GET | `/api/channel/test` | `channel.operate` | 触发全量渠道测试系统任务 | `channel-router.go:47` |
| GET | `/api/channel/test/:id` | `channel.operate` | 测试单个渠道 | `channel-router.go:48` |
| GET | `/api/channel/update_balance` | `channel.operate` | 刷新全部渠道余额（同步、逐个上游调用） | `channel-router.go:49` |
| GET | `/api/channel/update_balance/:id` | `channel.operate` | 刷新单渠道余额 | `channel-router.go:50` |
| POST | `/api/channel/` | `channel.sensitive_write` | 新建渠道（支持 single/batch/multi_to_single） | `channel-router.go:51` |
| PUT | `/api/channel/` | `channel.write` | 更新渠道（含敏感字段时另需 `sensitive_write`） | `channel-router.go:52` |
| POST | `/api/channel/status/batch` | `channel.operate` | 批量启停渠道 | `channel-router.go:53` |
| POST | `/api/channel/:id/status` | `channel.operate` | 单渠道启停 | `channel-router.go:54` |
| DELETE | `/api/channel/disabled` | `channel.sensitive_write` | **删除全部已禁用渠道** | `channel-router.go:55` |
| POST | `/api/channel/tag/disabled` | `channel.operate` | 按标签禁用渠道 | `channel-router.go:56` |
| POST | `/api/channel/tag/enabled` | `channel.operate` | 按标签启用渠道 | `channel-router.go:57` |
| PUT | `/api/channel/tag` | `channel.write` | 按标签批量改属性 | `channel-router.go:58` |
| DELETE | `/api/channel/:id` | `channel.sensitive_write` | **删除单渠道** | `channel-router.go:59` |
| POST | `/api/channel/batch` | `channel.sensitive_write` | **批量删除渠道** | `channel-router.go:60` |
| POST | `/api/channel/fix` | `channel.operate` | 修复渠道-能力表 | `channel-router.go:61` |
| GET | `/api/channel/fetch_models/:id` | `channel.operate` | 拉取指定渠道的上游模型 ID 列表 | `channel-router.go:62` |
| GET | `/api/channel/:id/vllm/status` | `channel.read` | vLLM 渠道状态（`controller/channel_inference.go:49`） | `channel-router.go:63` |
| GET | `/api/channel/:id/sglang/status` | `channel.read` | SGLang 渠道状态（`controller/channel_inference.go:53`） | `channel-router.go:64` |
| POST | `/api/channel/fetch_models` | `channel.sensitive_write` | 用请求体中的 key/base_url 探测上游模型 | `channel-router.go:65` |
| POST | `/api/channel/:id/codex/refresh` | `channel.sensitive_write` | 刷新 Codex 渠道 OAuth 凭据 | `channel-router.go:66` |
| GET | `/api/channel/:id/codex/usage` | `channel.read` | 查询 Codex 渠道用量 | `channel-router.go:67` |
| GET | `/api/channel/:id/codex/usage/reset-credits` | `channel.read` | 查询 Codex 重置次数 | `channel-router.go:68` |
| POST | `/api/channel/:id/codex/usage/reset` | `channel.operate` | **消耗一次 Codex 重置额度** | `channel-router.go:69` |
| POST | `/api/channel/ollama/pull` | `channel.sensitive_write` | 拉取 Ollama 模型 | `channel-router.go:70` |
| POST | `/api/channel/ollama/pull/stream` | `channel.sensitive_write` | 流式拉取（SSE） | `channel-router.go:71` |
| DELETE | `/api/channel/ollama/delete` | `channel.sensitive_write` | **删除 Ollama 模型** | `channel-router.go:72` |
| GET | `/api/channel/ollama/version/:id` | `channel.sensitive_write` | 查询 Ollama 版本 | `channel-router.go:73` |
| POST | `/api/channel/batch/tag` | `channel.write` | 批量为渠道设置标签 | `channel-router.go:74` |
| GET | `/api/channel/tag/models` | `channel.read` | 取该标签下最长的模型串 | `channel-router.go:75` |
| POST | `/api/channel/copy/:id` | `channel.sensitive_write` | **复制渠道（含密钥）** | `channel-router.go:76` |
| POST | `/api/channel/multi_key/manage` | `channel.operate` | 多密钥渠道管理（部分动作另需 `sensitive_write`） | `channel-router.go:77` |
| POST | `/api/channel/upstream_updates/apply` | `channel.write` | 应用单渠道上游模型变更 | `channel-router.go:78` |
| POST | `/api/channel/upstream_updates/apply_all` | `channel.write` | 应用全部渠道上游模型变更 | `channel-router.go:79` |
| POST | `/api/channel/upstream_updates/detect` | `channel.operate` | 探测单渠道上游模型变更 | `channel-router.go:80` |
| POST | `/api/channel/upstream_updates/detect_all` | `channel.operate` | 探测全部渠道上游模型变更 | `channel-router.go:81` |
| POST | `/api/channel/:id/key` | **RootAuth** | 查看渠道完整密钥 | `channel-router.go:23` |

**凭据与破坏性提示**

- `POST /api/channel/:id/key` 是唯一查看渠道明文密钥的接口。它绕过 `RequirePermission`，改用 `RootAuth` + `CriticalRateLimit` + `DisableCache` + `SecureVerificationRequired`（`channel-router.go:23`）。成功返回 `data.key`（`controller/channel.go:531`），并记录 `channel.key_view` 审计（`controller/channel.go:525`）。
- 列表与详情接口通过 `.Omit("key")`（`controller/channel.go:231`、`controller/channel.go:250`）与 `clearChannelInfo`（`controller/channel.go:69`）脱敏：多密钥渠道的禁用原因与禁用时间被置空。因此**列表响应中渠道的 `key` 字段不出现**。
- 破坏性：`DELETE /api/channel/disabled`（清空全部已禁用渠道）、`DELETE /api/channel/:id`、`POST /api/channel/batch`、`DELETE /api/channel/ollama/delete`、`POST /api/channel/:id/codex/usage/reset`。
- `POST /api/channel/copy/:id` 会复制原渠道密钥（`controller/channel.go:1571`）；query 参数 `suffix`（默认 `_复制`）与 `reset_balance`（默认 `true`，`controller/channel.go:1579`）。

### 渠道：关键请求体

**POST /api/channel/**（`controller/channel.go:708`）：请求体 `AddChannelRequest`（`controller/channel.go:669`）：`mode`（`single`/`batch`/`multi_to_single`，其他值返回「不支持的添加模式」）、`multi_key_mode`、`batch_add_set_key_prefix_2_name`、`channel`（`model.Channel`）。

- 渠道类型为任务插件（`constant.ChannelTypeTaskPlugin`）时，额外要求调用方具备 `task_plugin.bind`，否则返回 HTTP 200 + 失败消息（`controller/channel.go:716`）。
- 校验规则见 `validateChannel`（`controller/channel.go:541`）：任务插件渠道必须有 1–30 字符的 `setting.task_plugin_key` 且插件已注册；New API / vLLM / SGLang 渠道 base URL 不得为空；新增时 `key` 不得为空、模型名 ≤ 255；VertexAI 需 `other` 含 `default` 地区；Codex 渠道的 `key` 必须是含 `access_token` 与 `account_id` 的 JSON 对象。

**PUT /api/channel/**（`controller/channel.go:1074`）：请求体 `PatchChannel`（`controller/channel.go:1059`）= `model.Channel` + `multi_key_mode` + `key_mode`（`append`/`replace`）。

- 请求体含 `status` 字段时直接拒绝（`controller/channel.go:1090`），状态必须走 `/status` 接口。
- 变更 `key`、`base_url`、`param_override`、`header_override` 等敏感字段时，会额外检查 `channel.sensitive_write`（`controller/channel.go:1143`）；不满足返回权限不足。
- `key_mode: "append"` 在 `channel_info.is_multi_key` 为真时把新 key 追加到现有列表并去重（`controller/channel.go:1155`）。
- 成功返回的 `data` 中 `key` 被清空（`controller/channel.go:1269`），并返回 `clearChannelInfo` 处理后的渠道。
- 审计只记录变更字段名与「key 已更换」，**绝不记录 key 内容**（`controller/channel.go:1243`）。

**GET /api/channel/ops**（`controller/channel.go:158`）：可选 query `auto_ban`（bool）。返回 `data.retry_times`、`data.request_policy.automatic_disable`、`data.request_policy.source`（`global`/`channel`/`global_and_channel`）。

**GET /api/channel/**（`controller/channel.go:191`）：query `p`、`page_size`、`id_sort`、`sort_by`、`sort_order`、`tag_mode`（按标签分组返回）、`group`、`status`（`enabled`/`1`/`disabled`/`0`，其他值表示全部，`controller/channel.go:58`）、`type`（渠道类型整数）。返回 `data.items`、`data.total`、`data.page`、`data.page_size`、`data.type_counts`（类型 → 数量）。

**GET /api/channel/search**（`controller/channel.go:366`）：query `keyword`、`group`、`model`、`status`、`id_sort`、`sort_by`、`sort_order`、`tag_mode`、`type`、`p`、`page_size`（后两者用 `c.DefaultQuery`，默认 `1`/`20`，`controller/channel.go:444`）。返回 `data.items`、`data.total`、`data.type_counts`（注意：**不含 `page`/`page_size`**）。

**POST /api/channel/multi_key/manage**（`controller/channel.go:1665`）：请求体 `MultiKeyManageRequest`（定义在 `controller/channel.go` 内，**字段未逐字段核对**，至少含 `channel_id`、`action`、`page`、`page_size`）。`get_key_status` 为只读且不写审计；`delete_key`、`delete_disabled_keys`、`update_rules` 额外要求 `channel.sensitive_write`（`controller/channel.go:2171`）。渠道非多密钥模式时返回「该渠道不是多密钥模式」。

**POST /api/channel/upstream_updates/apply**（`controller/channel_upstream_update.go:903`）：请求体含 `id`（必填，> 0）、`add_models`、`remove_models`、`ignore_models`。返回 `data.id`、`added_models`、`removed_models`、`ignored_models`、`remaining_models`、`remaining_remove_models`、`models`、`settings`（`controller/channel_upstream_update.go:939`）。

**POST /api/channel/upstream_updates/detect**（`controller/channel_upstream_update.go:959`）：请求体含 `id`。返回 `data.channel_id`、`channel_name`、`add_models`、`remove_models`、`last_check_time`、`auto_added_models`（`controller/channel_upstream_update.go:988`）。

**POST /api/channel/fetch_models**（`controller/channel.go:1436`）：请求体 `fetchModelsRequest`（**字段未逐字段核对**，引用到 `type`、`channel_id`、`base_url`、`key`）。Codex 类型之外的 key 只取第一行（`controller/channel.go:1471`）。

**POST /api/channel/ollama/pull**（`controller/channel.go:2190`）、**`/ollama/pull/stream`**（`controller/channel.go:2253`）、**`DELETE /api/channel/ollama/delete`**（`controller/channel.go:2335`）：请求体 `{channel_id, model_name}`；渠道类型不是 Ollama 时返回「This operation is only supported for Ollama channels」（`controller/channel.go:2220`）。stream 版本返回 `text/event-stream`，以 `data: [DONE]` 结束（`controller/channel.go:2319`）。

### 用户（users）

路由声明于 `api-router:165`（`adminRoute`，全部 `AdminAuth`）。

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/user/` | 分页用户列表 | `api-router:168` |
| GET | `/api/user/topup` | 全平台充值记录 | `api-router:169` |
| POST | `/api/user/topup/complete` | **管理员补单** | `api-router:170` |
| GET | `/api/user/search` | 按关键字/分组/角色/状态搜索用户 | `api-router:171` |
| GET | `/api/user/:id/oauth/bindings` | 查看指定用户的 OAuth 绑定 | `api-router:172` |
| DELETE | `/api/user/:id/oauth/bindings/:provider_id` | **解绑指定用户的自定义 OAuth** | `api-router:173` |
| DELETE | `/api/user/:id/bindings/:binding_type` | **清除指定用户的某类绑定** | `api-router:174` |
| GET | `/api/user/:id` | 单个用户详情 | `api-router:175` |
| POST | `/api/user/` | 新建用户 | `api-router:176` |
| POST | `/api/user/manage` | 单用户操作（启停/删除/升降级/加额度） | `api-router:177` |
| POST | `/api/user/manage/batch` | **批量启停/删除用户** | `api-router:178` |
| PUT | `/api/user/` | 更新用户 | `api-router:179` |
| DELETE | `/api/user/:id` | **删除用户（硬删除）** | `api-router:180` |
| DELETE | `/api/user/:id/reset_passkey` | **重置用户 Passkey** | `api-router:181` |
| GET | `/api/user/2fa/stats` | 2FA 统计 | `api-router:184` |
| DELETE | `/api/user/:id/2fa` | **强制禁用用户 2FA** | `api-router:185` |

**层级保护**：几乎所有用户管理接口都要求「可管理目标角色」，即调用者是 root，或角色严格大于目标角色（`controller/user.go:424`）：

- `GET /api/user/:id` 用 `canManageTargetRole`（`controller/user.go:440`），不满足返回 `MsgUserNoPermissionSameLevel`。返回的 `data` 附加 `admin_permissions`（`authz.Capabilities`，`controller/user.go:444`）。
- `DELETE /api/user/:id` 用更严格的 `myRole <= originUser.Role` 判断（`controller/user.go:961`），即**管理员不能删除同级**；root 用户不可被删除由 `HardDeleteUserById` 兜底。
- `POST /api/user/manage`（`controller/user.go:1177`）：请求体 `ManageRequest`（`controller/user.go:1082`）：`id`、`action`、`value`、`mode`。`action` 取值：`add_quota`（走 `manageUserQuota`）、`disable`、`enable`、`delete`、`promote`、`demote`。`promote`/`demote` 仅 root 可执行（`controller/user.go:1240`）；`disable`/`delete` 对 root 用户被拒（`controller/user.go:1206`、`controller/user.go:1213`）；`demote` 会同时清空该用户的授权策略并撤销其全部会话（`controller/user.go:1264`）。
- `POST /api/user/manage/batch`（`controller/user.go:1097`）：请求体 `BatchManageRequest`（`controller/user.go:1089`）：`ids`（1–100 个，`controller/user.go:1103`）、`action`（仅 `enable`/`disable`/`delete`）。`delete` 走 `HardDeleteUserById`。返回 `data` 为处理条数。
- `POST /api/user/`（`controller/user.go:1008`）：请求体 `model.User` 字段（`username`、`password`、`display_name`、`role`、`admin_permissions` 等）。`role` 必须严格小于调用者角色（`controller/user.go:1024`）。**`admin_permissions` 仅 root 可设置**（`controller/user.go:1073`），且仅对角色 ≥ 管理员的用户生效（`controller/user.go:1076`）。
- `PUT /api/user/`（`controller/user.go:683`）：请求体 `model.User`（需含 `id`）。`role` 只允许等于原角色或 `RoleGuestUser`（`controller/user.go:704`），随后被强制回写为原角色（`controller/user.go:708`），因此该接口**不能改角色**。若请求中的 `auth_version` 大于库中值，会撤销该用户全部会话（`controller/user.go:733`）。
- `DELETE /api/user/:id/reset_passkey`（`controller/passkey.go:443`）：目标未绑定 Passkey 时返回失败「该用户尚未绑定 Passkey」；成功后撤销该用户全部会话（`controller/passkey.go:474`）。
- `DELETE /api/user/:id/2fa`（`controller/twofa.go:232`）：目标未启用 2FA 时返回失败「用户未启用2FA」；成功后撤销全部会话（`controller/twofa.go:275`）。
- `POST /api/user/topup/complete`（`controller/topup.go:784`）：请求体 `{"trade_no":"<订单号>"}`，按订单号互斥补单（`controller/topup.go:792`）。
- `DELETE /api/user/:id/bindings/:binding_type`（`controller/user.go:754`）：`binding_type` 经小写去空格后传给 `user.ClearBinding`，非空即可，具体取值由 model 决定（**取值集合未核对**）。

### 模型与供应商

**供应商 `/api/vendors`**（`api-router:452`，`AdminAuth`）：

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| POST | `/api/vendors/operations/preview` | 预览供应商合并/迁移操作 | `api-router:455` |
| POST | `/api/vendors/operations` | **执行供应商合并/迁移** | `api-router:456` |
| GET | `/api/vendors/` | 分页列出（内部转发到 search） | `api-router:457` |
| GET | `/api/vendors/search` | 按关键字/关联关系搜索 | `api-router:458` |
| GET | `/api/vendors/:id` | 单个供应商 | `api-router:459` |
| POST | `/api/vendors/` | 新建供应商 | `api-router:460` |
| PUT | `/api/vendors/` | 更新供应商（需 `id`） | `api-router:461` |
| DELETE | `/api/vendors/:id` | **删除供应商** | `api-router:462` |

`GET /api/vendors/` 与 `/search` 返回相同的分页结构，query 支持 `keyword`、`association`（`controller/vendor_meta.go:17`）。错误处理走 `vendorAPIError`（`controller/vendor_meta.go:95`）：默认 HTTP 400；`ErrVendorConflict` → HTTP 409 + `code: "VENDOR_CONFLICT"`；被引用时 → HTTP 409 + `code: "VENDOR_REFERENCED"` + `reference_counts`。`POST /api/vendors/operations` 请求体 `model.VendorOperation`（`model/vendor_management.go:79`）：`action`、`vendor_ids`、`model_ids`、`target_vendor_id`、`expected_version`。

**模型 `/api/models`**（`api-router:465`，`AdminAuth`）：

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/models/sync_upstream/preview` | 预览上游元数据同步（query `locale`） | `api-router:468` |
| POST | `/api/models/sync_upstream` | 应用上游元数据同步 | `api-router:469` |
| POST | `/api/models/delete` | **批量删除模型元数据** | `api-router:470` |
| GET | `/api/models/missing` | 列出缺失元数据的模型 | `api-router:471` |
| DELETE | `/api/models/delete-unused` | **删除未被任何渠道引用的模型元数据** | `api-router:472` |
| GET | `/api/models/` | 分页列出模型元数据 | `api-router:473` |
| GET | `/api/models/search` | 按关键字/供应商搜索 | `api-router:474` |
| GET | `/api/models/:id` | 单个模型元数据 | `api-router:475` |
| POST | `/api/models/` | 新建模型元数据 | `api-router:476` |
| PUT | `/api/models/` | 更新模型元数据 | `api-router:477` |
| DELETE | `/api/models/:id` | **删除模型元数据** | `api-router:478` |

- 列表 query：`keyword`、`vendor`、`status`、`sync_official`、`include_channel_models`（`=true` 时改用带渠道的查询，`controller/model_meta.go:50`）、`square_state`（`visible`/`unavailable`/`hidden`/`partial`，非法值返回 HTTP 400，`controller/model_meta.go:27`）。返回体不是标准 `PageInfo`：直接返回 `{"items","total","page","page_size","vendor_counts"}`（`controller/model_meta.go:81`）。
- `POST /api/models/`（`controller/model_meta.go:111`）与 `PUT /api/models/`（`controller/model_meta.go:144`）：请求体 `model.Model`（`model/model_meta.go:35`）。`model_name` 必填且不得重名。`PUT` 支持 `?status_only=true` 只改 `status`（值须为 0 或 1，`controller/model_meta.go:157`）。
- `DELETE /api/models/:id`（`controller/model_meta.go:196`）与 `POST /api/models/delete`（`controller/model_meta.go:226`）：query/body 支持 `remove_from_channels`（默认 false）与 `remove_pricing`（默认 false）。**`remove_pricing=true` 仅 root 可执行**，否则返回 HTTP 403 + `"Model pricing is managed by a super administrator."`（`controller/model_meta.go:213`、`controller/model_meta.go:236`）。
- `POST /api/models/sync_upstream`（`controller/model_sync.go:401`）：请求体 `{locale, source_version, selections}`，缺任一必填项返回 HTTP 400；`source_version` 与服务端不一致或所选模型已下架返回 HTTP 409（`controller/model_sync.go:414`、`controller/model_sync.go:418`）。

### 模型部署（io.net，`/api/deployments`）

`api-router:482`，全部 `AdminAuth`。所有接口依赖已配置的 io.net API key（`model_deployment.ionet.enabled` + `model_deployment.ionet.api_key`），未配置时返回失败「io.net model deployment is not enabled or api key missing」（`controller/deployment.go:22`）。

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/deployments/settings` | 返回 `provider`、`enabled`、`configured`、`can_connect` | `api-router:485` |
| POST | `/api/deployments/settings/test-connection` | 测试 API key（可传 `api_key`，否则用已存值） | `api-router:486` |
| GET | `/api/deployments/` | 分页列出部署（query `status`） | `api-router:487` |
| GET | `/api/deployments/search` | 按 `status`/`keyword` 搜索 | `api-router:488` |
| POST | `/api/deployments/test-connection` | 同上测试连接 | `api-router:489` |
| GET | `/api/deployments/hardware-types` | 硬件类型列表 | `api-router:490` |
| GET | `/api/deployments/locations` | 可用地域 | `api-router:491` |
| GET | `/api/deployments/available-replicas` | query `hardware_id`（必填）、`gpu_count`（默认 1） | `api-router:492` |
| POST | `/api/deployments/price-estimation` | 价格估算 | `api-router:493` |
| GET | `/api/deployments/check-name` | query `name`（必填），检查集群名可用性 | `api-router:494` |
| POST | `/api/deployments/` | **创建部署** | `api-router:495` |
| GET | `/api/deployments/:id` | 部署详情 | `api-router:497` |
| GET | `/api/deployments/:id/logs` | 容器日志（query `container_id` 必填、`level`、`stream`、`cursor`、`limit`≤1000、`follow`、`start_time`/`end_time` RFC3339） | `api-router:498` |
| GET | `/api/deployments/:id/containers` | 容器列表 | `api-router:499` |
| GET | `/api/deployments/:id/containers/:container_id` | 容器详情 | `api-router:500` |
| PUT | `/api/deployments/:id` | 更新部署 | `api-router:501` |
| PUT | `/api/deployments/:id/name` | 改集群名（先查可用性，不可用则拒绝） | `api-router:502` |
| POST | `/api/deployments/:id/extend` | 延长部署时长 | `api-router:503` |
| DELETE | `/api/deployments/:id` | **终止部署** | `api-router:504` |

`POST /api/deployments/` 请求体为 `ionet.DeploymentRequest`（外部 SDK 类型，**字段未核对**）；`PUT /api/deployments/:id` 为 `ionet.UpdateDeploymentRequest`；`POST /api/deployments/:id/extend` 为 `ionet.ExtendDurationRequest`。

### 系统选项（`/api/option`）

`api-router:248`，**全部 `RootAuth`**（`api-router:249`）。

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/option/` | 读取全部选项（敏感项以哨兵值返回） | `api-router:251` |
| GET | `/api/option/request_policy` | 读取请求策略选项 | `api-router:252` |
| PATCH | `/api/option/request_policy` | 更新请求策略选项 | `api-router:253` |
| PUT | `/api/option/` | 更新单个选项 | `api-router:254` |
| PUT | `/api/option/passkey/domains` | 更新 Passkey RP ID 与 origins | `api-router:255` |
| GET | `/api/option/model_pricing` | 读取模型计价配置（query `model` 可重复） | `api-router:256` |
| PATCH | `/api/option/model_pricing` | 更新模型计价配置 | `api-router:257` |
| POST | `/api/option/smtp/test` | 发送 SMTP 测试邮件 | `api-router:258` |
| DELETE | `/api/option/support` | **清空站内支持数据** | `api-router:259` |
| POST | `/api/option/model_pricing/preview` | 预览计价计算结果 | `api-router:260` |
| POST | `/api/option/payment_compliance` | 确认支付合规声明 | `api-router:261` |
| GET | `/api/option/channel_affinity_cache` | 渠道亲和缓存统计 | `api-router:262` |
| DELETE | `/api/option/channel_affinity_cache` | **清空渠道亲和缓存** | `api-router:263` |
| POST | `/api/option/rest_model_ratio` | **重置模型倍率为默认值** | `api-router:264` |
| GET | `/api/option/waffo-pancake/catalog` | 查询 Waffo Pancake 目录 | `api-router:265` |
| POST | `/api/option/waffo-pancake/pair` | 配对 Waffo Pancake | `api-router:266` |
| POST | `/api/option/waffo-pancake/save` | 保存 Waffo Pancake 配置 | `api-router:267` |
| POST | `/api/option/waffo-pancake/subscription-product` | 创建 Waffo Pancake 订阅商品 | `api-router:268` |
| GET | `/api/option/waffo-pancake/subscription-product-options` | 列出可绑定的订阅商品选项 | `api-router:269` |

**GET /api/option/**（`controller/option.go:87`）关键行为：

- 跳过 `theme.frontend`、`billing_setting.billing_mode`、`billing_setting.billing_expr`（`controller/option.go:92`），其中后两者在函数末尾以「生效值」重新追加（`controller/option.go:144`）。
- `EpayGateways` 的每个网关 `key` 被替换为哨兵（`controller/option.go:96`）。
- 敏感键判定（`controller/option.go:111`）：非 `*SiteKey` 结尾，且满足 `MoneroWalletRPCPassword`、或后缀 `Token`/`Secret`/`Key`/`secret`/`api_key`。命中时若原值非空则返回 `common.SensitiveOptionPlaceholder`，否则返回空串（`controller/option.go:117`）。**这是「已配置」与「从未设置」的区分哨兵，不是真实值。**
- 额外追加 `CompletionRatioMeta`（`controller/option.go:155`）。

**PUT /api/option/**（`controller/option.go:281`）：请求体 `OptionUpdateRequest`（`controller/option.go:166`）：`key`、`value`（任意类型，内部转字符串，`controller/option.go:291`）。

- 提交的值等于哨兵时**直接返回成功且不写入**（`controller/option.go:309`），防止陈旧客户端用占位符覆盖真实密钥。
- `EpayGateways` 会校验：值为哨兵的网关必须匹配已存在的网关 ID，否则拒绝（`controller/option.go:313`）。
- 按 `key` 分派大量校验（`controller/option.go:340` 起）：`CaptchaType` 只接受 `turnstile`/`hcaptcha`/`cap`；Monero 系列有网络/确认数/子地址上限/汇率校验；NOWPayments 系列有 URL、币种（≤50、字符集受限）、最小充值、汇率、过期分钟数校验；`LoginCaptchaDifficulty`/`CheckinCaptchaDifficulty` 限 1–8；`CustomTabs` ≤ 50 条且 URL 必须是 `/` 开头或 http(s)；`StatusCheckGroups` ≤ 100 条、`StatusCheckCacheExcludedModels` ≤ 500 条；`QuotaForInviter`/`QuotaForInvitee` 在未确认支付合规时若为正数则拒绝（`controller/option.go:549`）。
- 以 `payment_setting.compliance_` 为前缀的键**不允许**通过本接口修改（`controller/option.go:555`），必须走 `POST /api/option/payment_compliance`。
- 启用类开关有前置配置检查，例如 `GitHubOAuthEnabled` 需先填 Client ID、`discord.enabled`、`oidc.enabled`、`LinuxDOOAuthEnabled`、`EmailDomainRestrictionEnabled`、`WeChatAuthEnabled`、`PhantomCheckEnabled`、`TurnstileCheckEnabled`、`HCaptchaEnabled`、`CapEnabled`、`ForceCheckinCaptcha`、`ForceRedemptionCaptcha`、`TelegramOAuthEnabled`（`controller/option.go:566` 起）。
- `theme.frontend` 只接受 `default`（`controller/option.go:677`）。
- 若 `key` 是 Passkey 域选项，走 `UpdatePasskeyDomainOptions` 并记录 Passkey 域审计（`controller/option.go:890`）。
- 审计只记录被修改的**键名**，不记录值（`controller/option.go:910`）。

**PUT /api/option/passkey/domains**（`controller/option.go:237`）：请求体 `{rp_id, legacy_rp_ids, origins, preview, removal_confirmation}`，四个字段中前三者必须非 null（`controller/option.go:245`）。`preview: true` 时只预演、不写入也不写审计。移除 RP ID 未提供确认串时返回 **HTTP 409** + `code: "PASSKEY_RP_ID_REMOVAL_CONFIRMATION_REQUIRED"` + `data`（`controller/option.go:268`）。

**GET|PATCH /api/option/model_pricing**（`controller/model_pricing_config.go:9`、`controller/model_pricing_config.go:48`）：`GET` 的 query `model` 可重复。`PATCH` 请求体 `{"changes":[{"model_name":..., "pricing":{...}}]}`（`controller/model_pricing_config.go:49`）；冲突时返回 HTTP 409（`controller/model_pricing_config.go:56`）。

**POST /api/option/model_pricing/preview**（`controller/model_pricing_config.go:19`）：请求体 `{model_name, pricing}`，返回 `ModelPricingDescription`（`effective`、`cache_write_mode`、`billing_details`）。

**POST /api/option/payment_compliance**（`controller/payment_compliance.go:30`）：请求体含 `confirmed`（bool）。使用 PAT 认证时返回 **HTTP 403**（`controller/payment_compliance.go:31`）。`confirmed=false` 时拒绝（`controller/payment_compliance.go:40`）。写入 5 个 `payment_setting.compliance_*` 选项（`controller/payment_compliance.go:49`）。

**DELETE /api/option/channel_affinity_cache**（`controller/channel_affinity_cache.go:20`）：query `all=true` 清空全部；否则需 `rule_name`，缺失返回 HTTP 400「缺少参数：rule_name，或使用 all=true 清空全部」（`controller/channel_affinity_cache.go:39`）。

**POST /api/option/rest_model_ratio**（`controller/pricing.go:102`）：无请求体，把 `ModelRatio` 重置为内置默认并返回中文提示。**破坏性（覆盖运营方自定义倍率）。**

**GET /api/option/request_policy**（`controller/request_policy.go:10`）：返回 `data.options`。**PATCH**（`controller/request_policy.go:14`）：请求体 `{"options": {"<key>": "<value>"}}`。

**POST /api/option/smtp/test**（`controller/option_email.go:17`）：请求体 `{"email":"..."}`，邮箱非法时返回参数错误。

### 日志与审计

| 方法 | 路径 | 角色/权限 | 用途 | 行号 |
| --- | --- | --- | --- | --- |
| GET | `/api/log/` | AdminAuth | 全平台日志（分页） | `api-router:384` |
| GET | `/api/log/export` | AdminAuth | 导出全平台日志 CSV | `api-router:385` |
| GET | `/api/log/stat` | AdminAuth | 全平台日志统计 | `api-router:386` |
| GET | `/api/log/search` | AdminAuth | **已废弃**，固定返回失败（`controller/log.go:116`） | `api-router:389` |
| GET | `/api/log/channel_affinity_usage_cache` | AdminAuth | 渠道亲和用量缓存统计（query `rule_name` 必填、`using_group`、`key_fp`） | `api-router:388` |
| GET | `/api/audit` | AdminAuth + `audit.read` | 其他账号审计日志 | `api-router:379` |
| GET | `/api/audit/export` | AdminAuth + `audit.read` | 导出审计 CSV | `api-router:380` |
| GET | `/api/data/` | AdminAuth | 全平台额度消耗按日聚合 | `api-router:412` |
| GET | `/api/data/users` | AdminAuth | 按用户聚合的额度消耗 | `api-router:413` |
| GET | `/api/data/flow` | AdminAuth | 全平台流量额度按日聚合 | `api-router:415` |

- `GET /api/log/`（`controller/log.go:17`）：query `type`、`start_timestamp`、`end_timestamp`、`username`、`token_name`、`model_name`、`channel`、`group`、`request_id`、`upstream_request_id`。root 与非 root 走不同的格式化（`controller/log.go:34`）：非 root 用 `FormatAdminLogs`，root 用 `FormatRootLogs`。
- `GET /api/log/stat`（`controller/log.go:155`）：query 同上去掉 `request_id`，返回 `data.quota`、`data.rpm`、`data.tpm`。
- `GET /api/log/export`（`controller/log.go:67`）：`selfView` 仅在路径为 `/api/log/self/export` 时为真，因此本接口导出全平台。响应为 CSV 附件 `usage-logs-<时间戳>.csv`。
- `GET /api/audit`（`controller/access_token.go:68`）：query 与用户端 `/api/audit/self` 相同（`category`、`token_ref`、`exclude_token_ref`、`request_id`、`start_timestamp`、`end_timestamp`、`success`、`username`）。`selfView` 判定路径为 `/api/audit/self`，因此本接口按 query 中的 `username` 过滤。**root 的审计记录始终被排除**（`service/authz/resources_audit.go:13`）。分页 `page` 上限校验为 100000000（`controller/access_token.go:70`）。
- `GET /api/data/`（`controller/usedata.go:31`）：query `start_timestamp`、`end_timestamp`、`username`。
- `GET /api/data/users`（`controller/usedata.go:48`）：query `start_timestamp`、`end_timestamp`，按用户分组。
- `GET /api/data/flow`（`controller/usedata.go:88`）：query `start_timestamp`、`end_timestamp` **必填为正数**，否则返回 `invalid start_timestamp`/`invalid end_timestamp`/`invalid time range`（`controller/usedata.go:13`）；`username` 可选。

### 兑换码（`/api/redemption`）

`api-router:367`，全部 `AdminAuth`。

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/redemption/` | 分页列出（query `creator_type`） | `api-router:370` |
| GET | `/api/redemption/search` | 搜索（query `keyword`、`status`、`creator_type`） | `api-router:371` |
| GET | `/api/redemption/:id` | 单个兑换码 | `api-router:372` |
| POST | `/api/redemption/` | 批量生成兑换码 | `api-router:373` |
| PUT | `/api/redemption/` | 更新兑换码 | `api-router:374` |
| POST | `/api/redemption/batch` | **批量删除兑换码** | `api-router:375` |
| DELETE | `/api/redemption/invalid` | **删除全部失效兑换码** | `api-router:376` |
| DELETE | `/api/redemption/:id` | **删除单个兑换码** | `api-router:377` |

- `POST /api/redemption/`（`controller/redemption.go:66`）：未确认支付合规时拒绝（`controller/redemption.go:67`）。请求体 `model.Redemption`（`model/redemption.go:17`）。校验：`name` 长度 1–20（`controller/redemption.go:78`）；`count` 1–100（`controller/redemption.go:82`、`controller/redemption.go:86`）；若 `subscription_plan_id > 0` 则套餐必须存在且启用，且 `quota` 被强制为 0（`controller/redemption.go:90`）；否则 `quota` 必须为正且通过 `common.ValidateWalletQuota`（`controller/redemption.go:102`）；`expired_time` 若不为 0 必须大于当前时间（`controller/redemption.go:278`）。成功返回 `data` 为生成的 key 数组（`controller/redemption.go:146`）。
- `PUT /api/redemption/`（`controller/redemption.go:189`）：请求体 `{id, status, name, quota, subscription_plan_id, expired_time}`。`?status_only=<非空>` 时只改 `status`（`controller/redemption.go:248`）。`subscription_plan_id` 是指针类型，用于区分「未传」与「传 0」。
- `POST /api/redemption/batch`（`controller/redemption.go:168`）：请求体 `{"ids":[...]}`，1–1000 个且每个 > 0（binding 校验），返回 `data` 为删除条数。
- `DELETE /api/redemption/invalid`（`controller/redemption.go:264`）：返回 `data` 为删除行数。

### 订阅（`/api/subscription/admin`）

`api-router:203`，全部 `AdminAuth`。

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/subscription/admin/plans` | 列出全部套餐（含禁用） | `api-router:206` |
| POST | `/api/subscription/admin/plans` | 新建套餐 | `api-router:207` |
| PUT | `/api/subscription/admin/plans/:id` | 更新套餐 | `api-router:208` |
| PATCH | `/api/subscription/admin/plans/:id` | 只改启用状态 | `api-router:209` |
| DELETE | `/api/subscription/admin/plans/:id` | **删除套餐** | `api-router:210` |
| POST | `/api/subscription/admin/bind` | 给用户绑定订阅 | `api-router:211` |
| POST | `/api/subscription/admin/plans/:id/subscriptions/reset` | **重置该套餐下全部订阅额度** | `api-router:212` |
| GET | `/api/subscription/admin/users/:id/subscriptions` | 列出指定用户的订阅 | `api-router:215` |
| POST | `/api/subscription/admin/users/:id/subscriptions` | 给指定用户新建订阅 | `api-router:216` |
| POST | `/api/subscription/admin/users/:id/subscriptions/reset` | **重置该用户某套餐的订阅额度** | `api-router:217` |
| POST | `/api/subscription/admin/user_subscriptions/:id/invalidate` | **作废单条用户订阅** | `api-router:218` |
| DELETE | `/api/subscription/admin/user_subscriptions/:id` | **硬删除单条用户订阅** | `api-router:219` |

- 新建/更新套餐（`controller/subscription.go:203`、`controller/subscription.go:277`）均要求已确认支付合规（`controller/subscription.go:204`、`controller/subscription.go:278`）。请求体 `{"plan": {...}}`（`controller/subscription.go:199`）。校验：`title` 非空；`price_amount` ∈ [0, 9999]（`controller/subscription.go:218`、`controller/subscription.go:222`）；`five_hour_limit`/`weekly_limit`/`monthly_limit` ∈ [0, `common.MaxQuota`]；`upgrade_group`/`downgrade_group` 若非空必须存在于分组倍率中（`controller/subscription.go:250`、`controller/subscription.go:257`）；`quota_reset_period` 为自定义时 `quota_reset_custom_seconds` 必须 > 0（`controller/subscription.go:264`）。`total_amount < 0` 表示无限额度，此时三个限额被清零（`controller/subscription.go:40`）。
- `PUT` 使用显式 `updateMap`（`controller/subscription.go:344`），因此**可以写入零值**；`allow_balance_pay` 与 `allow_wallet_overflow` 只在非 nil 时更新（`controller/subscription.go:372`）。
- `PATCH`（`controller/subscription.go:395`）：请求体 `{"enabled": <bool>}`，为 nil 时拒绝。
- `POST /bind`（`controller/subscription.go:440`）与 `POST /users/:id/subscriptions`（`controller/subscription.go:528`）：请求体 `{user_id, plan_id}` 或 `{plan_id}`，均调用 `model.AdminBindSubscription`，返回的 `data.message` 可能非空（表示已存在等提示，`controller/subscription.go:455`）。
- 重置类接口（`controller/subscription.go:555`、`controller/subscription.go:592`）：请求体 `AdminResetSubscriptionRequest`（`controller/subscription.go:482`）：`plan_id`、`advance_reset_time`、`reset_total`、`reset_five_hour`、`reset_weekly`。三个 `reset_*` 全为 nil 时默认四项全重置（`controller/subscription.go:498`）。`advance_reset_time` 为 nil 时默认 true（`controller/subscription.go:490`）。
- 作废（`controller/subscription.go:627`）与硬删除（`controller/subscription.go:646`）返回的 `data.message` 可能非空。

### 任务插件（`/api/plugin/task`）

`api-router:312`，**全部 `RootAuth`**（`api-router:313`）。

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/plugin/task` | 列出所有插件（工厂层 + 覆盖层） | `api-router:315` |
| POST | `/api/plugin/task` | 上传插件源码（等同 PUT） | `api-router:316` |
| PUT | `/api/plugin/task` | 上传插件源码（等同 POST） | `api-router:317` |
| GET | `/api/plugin/task/runtime/status` | 路由代际与最近重建结果 | `api-router:318` |
| GET | `/api/plugin/task/marketplace/sources` | 读取市场源配置 | `api-router:319` |
| PUT | `/api/plugin/task/marketplace/sources` | 更新市场源配置 | `api-router:320` |
| GET | `/api/plugin/task/:key` | 读取插件详情与源码 | `api-router:321` |
| GET | `/api/plugin/task/:key/icon` | 读取插件图标（图片） | `api-router:322` |
| GET | `/api/plugin/task/:key/versions` | 列出该 key 的全部版本 | `api-router:323` |
| POST | `/api/plugin/task/:key/activate` | **切换激活版本** | `api-router:324` |
| POST | `/api/plugin/task/:key/status` | **启用/禁用插件** | `api-router:325` |
| POST | `/api/plugin/task/:key/dryrun` | 在沙箱中调用插件钩子 | `api-router:326` |
| DELETE | `/api/plugin/task/:key/versions/:version` | **删除指定覆盖版本** | `api-router:327` |
| GET | `/api/task_plugin_options` | **AdminAuth + `task_plugin.bind`**，列出可绑定的插件选项 | `api-router:329` |

**注意 `task_plugin_options` 是这一组里唯一的例外**：它挂在 `AdminAuth` + `RequirePermission(authz.TaskPluginBind)` 下（`api-router:329`），而非 root-only。

- `POST|PUT /api/plugin/task`（`controller/task_plugin.go:50`）：请求体 `taskPluginUploadRequest`（`controller/task_plugin.go:39`）：`source`（必填，≤ 1 MiB，`controller/task_plugin.go:56`）、`enabled`、`remark`、`force`、`sourceSha256`（非空时校验 SHA-256，`controller/task_plugin.go:60`）、`icon`（data URI，须可解码，`controller/task_plugin.go:77`）。启用且未 `force` 时会做路由冲突预检（`controller/task_plugin.go:88`）。成功返回 `taskPluginDetail`：`plugin`、`meta`、`source`、`layer`（`override`）、`has_icon`（`controller/task_plugin.go:107`）。
- `GET /api/plugin/task`（`controller/task_plugin.go:152`）：返回 `taskPluginListItem` 数组（`controller/task_plugin.go:119`），含 `meta`、`source`（`factory`/`override`/`override_over_factory`）、`enabled`、`active`、`source_hash`、`has_icon`、`remark`、`runtime_status`、`runtime_error`、`factory_meta`、`channel_count`、`in_flight_count`。
- `POST /api/plugin/task/:key/status`（`controller/task_plugin.go:504`）：请求体 `{"enabled": <bool>}`。禁用时若仍有渠道在用且未传 `?cascade=true`，或仍有在途任务且未传 `?force=true`，返回 HTTP 200 + `{"success":false,"message":"task plugin is still in use","data":{"channels":[...],"in_flight_count":N}}`（`controller/task_plugin.go:520`）。`cascade=true` 会同时把相关渠道置为手动禁用并计数（`controller/task_plugin.go:524`）。成功返回 `data.plugin_enabled`、`data.disabled_channels`。
- `DELETE /api/plugin/task/:key/versions/:version`（`controller/task_plugin.go:420`）：工厂插件不可删除，返回「override plugin version not found; factory plugins cannot be deleted」。仍在使用且未传 `?force=true` 时返回 `"task plugin is still in use"`（`controller/task_plugin.go:438`）。
- `POST /api/plugin/task/:key/activate`（`controller/task_plugin.go:463`）：请求体 `{"version":"..."}`（必填），版本不存在返回「plugin version not found」。
- `POST /api/plugin/task/:key/dryrun`（`controller/task_plugin.go:378`）：请求体 `{hook（必填）, member, args}`（`controller/task_plugin.go:372`）。`args` 为 JSON 数组；`member` 为空时调用顶层 hook，否则调用成员方法（`controller/task_plugin.go:408`）。**该接口会在服务端执行插件 JS。**
- `PUT /api/plugin/task/marketplace/sources`（`controller/task_plugin.go:598`）：请求体为源数组，每项 `name` 非空、`index_url` 必须是绝对 http(s) URL（`controller/task_plugin.go:615`）。
- `GET /api/plugin/task/:key/icon`（`controller/task_plugin.go:313`）：非 JSON，直接返回图片字节，并设置 `X-Content-Type-Options: nosniff` 与 `Content-Security-Policy: default-src 'none'; ...; sandbox`（`controller/task_plugin.go:335`）。无图标返回 HTTP 404。

### 系统任务与实例（root）

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| POST | `/api/system-task/log-cleanup` | 创建日志清理系统任务（query `target_timestamp` 必填） | `api-router:397` |
| GET | `/api/system-task/list` | 列出系统任务（query `scope`、`type`、`status`、`offset`、`limit`） | `api-router:398` |
| DELETE | `/api/system-task/history` | **删除历史系统任务**（query 同 filter） | `api-router:399` |
| GET | `/api/system-task/current` | 查询某类型的当前活跃任务（query `type` 必填） | `api-router:400` |
| GET | `/api/system-task/:task_id` | 查询单个系统任务 | `api-router:401` |
| GET | `/api/system-info/instances` | 列出系统实例 | `api-router:406` |
| DELETE | `/api/system-info/stale-instances` | **删除全部过期实例记录** | `api-router:407` |
| DELETE | `/api/system-info/instances/:node_name` | **删除指定过期实例记录** | `api-router:408` |

全部 `RootAuth`（`api-router:395`、`api-router:404`）。

- `GET /api/system-task/list`（`controller/system_task.go:68`）：query 绑定 `model.SystemTaskFilter`（`model/system_task.go:191`）：`scope` ∈ {`active`, `history`}、`type`（≤ 64 字符）、`status` ∈ {`pending`, `running`, `succeeded`, `failed`}；另加 `offset`（≥ 0）、`limit`。绑定失败返回 HTTP 400 + `"invalid system task filters"`。响应含顶层 `total`（`controller/system_task.go:90`）。
- `DELETE /api/system-task/history`（`controller/system_task.go:98`）：返回 `data.deleted_count`。
- `GET /api/system-task/:task_id`（`controller/system_task.go:116`）：不存在返回 HTTP 404 + `"task not found"`。
- 任务响应体为 `model.SystemTaskResponse`（`model/system_task.go:64`）：`id`、`task_id`、`type`、`status`、`active_key`、`payload`、`state`、`result`、`error`、`locked_by`、`created_at`、`updated_at`。
- `DELETE /api/system-info/instances/:node_name`（`controller/system_info.go:45`）：实例非过期或不存在时返回失败「instance is not stale or no longer exists」。

### 令牌分组（AdminAuth）

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/token/group-names` | 令牌分组迁移的源/目标分组 | `api-router:335` |
| POST | `/api/token/group/migrate` | **把某分组下的全部令牌迁移到另一分组** | `api-router:336` |

- `GET`（`controller/token_group.go:20`）：返回 `data.source_groups`、`data.target_groups`。
- `POST`（`controller/token_group.go:41`）：请求体 `tokenGroupMigrationRequest`（含 `source_group`、`target_group`，**类型定义未核对**）。`target_group` 必须是当前可用分组，或为 `auto` 且已配置 auto 分组（`controller/token_group.go:49`），否则返回「target_group 不是当前可用分组」。返回 `data.source_group`、`target_group`、`migrated`（条数）。

### 分组与预填组（AdminAuth）

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/group/` | 全部分组名 | `api-router:425` |
| GET | `/api/prefill_group/` | 列出预填组（query `type`） | `api-router:431` |
| POST | `/api/prefill_group/` | 新建预填组 | `api-router:432` |
| PUT | `/api/prefill_group/` | 更新预填组（需 `id`） | `api-router:433` |
| DELETE | `/api/prefill_group/:id` | **删除预填组** | `api-router:434` |

- `GET /api/group/`（`controller/group.go:15`）：返回 `data` 为分组名数组。
- 预填组请求体 `model.PrefillGroup`（`model/prefill_group.go:78`）：`id`、`name`、`type`、`items`、`description`。新建时 `name` 与 `type` 均不得为空（`controller/prefill_group.go:30`），且名称不得重复（`controller/prefill_group.go:35`）。

### 授权目录（AdminAuth）

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/authz/catalog` | 返回权限资源/动作目录与角色基线矩阵 | `authz-router.go:17` |

`authz-router.go:15` 挂 `AdminAuth`。响应为 `data.resources`（`authz.Catalog()`）与 `data.roles`（`authz.Roles()`，`controller/authz.go:19`）。角色描述符含 `key`、`name`、`built_in`、`superuser`、`grants`（`service/authz/role.go:38`）。

### 站内支持（AdminAuth + SupportEnabled）

`api-router:232`。

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/support/admin/conversations` | 分页列出会话（query `keyword`） | `api-router:235` |
| GET | `/api/support/admin/conversations/:id` | 读取会话与消息 | `api-router:236` |
| POST | `/api/support/admin/conversations/:id/messages` | 回复消息/图片 | `api-router:237` |
| POST | `/api/support/admin/conversations/:id/grant-quota` | **直接发放额度** | `api-router:238` |
| POST | `/api/support/admin/conversations/:id/grant-subscription` | **直接发放订阅** | `api-router:239` |
| POST | `/api/support/admin/messages/:id/complete-order` | **手工补单某订单消息** | `api-router:240` |

- 发放额度（`controller/support.go:262`）：请求体 `{quota, note}`；`note` 为空时使用默认文案（`controller/support.go:280`）。发放后向目标用户写管理日志（`controller/support.go:287`）。
- 发放订阅（`controller/support.go:296`）：请求体 `{plan_id, note}`，`plan_id` 必填 > 0；同样要求已确认支付合规（`controller/support.go:297`）。
- 补单（`controller/support.go:333`）：目标消息必须是 `kind=order_quote`、`order_type=top_up`、`order_status=pending` 且 `trade_no` 非空，否则返回「该订单当前不可补单」（`controller/support.go:344`）。
- 消息发送（`controller/support.go:244`）：管理员不能直接用 `kind=quota_grant`/`subscription_grant` 发消息，必须走专用接口（`controller/support.go:186`）。

### 性能与倍率同步（root）

`api-router:296`、`api-router:306`，均 `RootAuth`。

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/performance/stats` | 磁盘缓存/内存/磁盘空间与配置快照 | `api-router:299` |
| DELETE | `/api/performance/disk_cache` | **清理超过 10 分钟未使用的磁盘缓存** | `api-router:300` |
| POST | `/api/performance/reset_stats` | **重置磁盘缓存统计** | `api-router:301` |
| POST | `/api/performance/gc` | 强制 GC | `api-router:302` |
| GET | `/api/performance/logs` | 日志文件列表 | `api-router:303` |
| DELETE | `/api/performance/logs` | **按数量或天数清理日志文件** | `api-router:304` |
| GET | `/api/ratio_sync/channels` | 可同步倍率的上游渠道列表 | `api-router:309` |
| POST | `/api/ratio_sync/fetch` | **从上游拉取倍率配置** | `api-router:310` |

- `DELETE /api/performance/logs`（`controller/performance.go:268`）：query `mode` 必须是 `by_count` 或 `by_days`，`value` 必须是正整数，否则返回错误；未配置日志目录时返回「log directory not configured」（`controller/performance.go:271`、`controller/performance.go:275`、`controller/performance.go:279`）。
- `GET /api/performance/logs`（`controller/performance.go:232`）：日志目录未配置时返回 `LogFilesResponse{Enabled:false}`。响应字段见 `controller/performance.go:225`。
- `GET /api/ratio_sync/channels`（`controller/ratio_sync.go:1107`）：返回 `dto.SyncableChannel` 数组，末尾追加两个内置预设：`officialRatioPresetID = -100`（官方倍率预设，`https://basellm.github.io`）与 `modelsDevPresetID = -101`（models.dev 价格预设，`https://models.dev`，`controller/ratio_sync.go:34`）。
- `POST /api/ratio_sync/fetch`（`controller/ratio_sync.go:211`）：请求体 `dto.UpstreamRequest`（含 `upstreams`、`channel_ids`、`timeout`；**逐字段定义未核对**）。`timeout <= 0` 时使用默认 10 秒（`controller/ratio_sync.go:218`）。`upstreams` 与 `channel_ids` 都为空时返回「无有效上游渠道」（`controller/ratio_sync.go:249`）。

### 自定义 OAuth 提供方（root）

`api-router:272`，`RootAuth`（`api-router:274`）。

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| POST | `/api/custom-oauth-provider/discovery` | 服务端抓取 OIDC Discovery 文档 | `api-router:276` |
| GET | `/api/custom-oauth-provider/` | 列出全部自定义提供方 | `api-router:277` |
| GET | `/api/custom-oauth-provider/:id` | 单个提供方 | `api-router:278` |
| POST | `/api/custom-oauth-provider/` | 新建提供方 | `api-router:279` |
| PUT | `/api/custom-oauth-provider/:id` | 更新提供方 | `api-router:280` |
| DELETE | `/api/custom-oauth-provider/:id` | **删除提供方** | `api-router:281` |

- `POST /discovery`（`controller/custom_oauth.go:159`）：请求体 `{well_known_url, issuer_url}`，两者都空时返回「请先填写 Discovery URL 或 Issuer URL」；仅给 issuer 时自动补 `/.well-known/openid-configuration`（`controller/custom_oauth.go:173`）。仅允许 http/https 且必须有 host（`controller/custom_oauth.go:179`）。超时 20 秒。返回 `data.well_known_url`、`data.discovery`。
- `POST /`（`controller/custom_oauth.go:238`）：请求体 `CreateCustomOAuthProviderRequest`（`controller/custom_oauth.go:126`），必填 `name`、`slug`、`client_id`、`client_secret`、`authorization_endpoint`、`token_endpoint`；可选 `icon`、`enabled`、`user_info_endpoint`、`identity_source`、`issuer`、`jwks_uri`、`scopes`、`user_id_field`、`username_field`、`display_name_field`、`email_field`、`well_known`、`auth_style`、`access_policy`、`access_denied_message`、`pkce_required`。slug 重复或与内置提供方冲突时拒绝（`controller/custom_oauth.go:245`、`controller/custom_oauth.go:249`）。**`client_secret` 属凭据；该文件处于合并冲突状态，`identity_source`/`issuer`/`jwks_uri` 三个字段仅存在于 HEAD 侧（`controller/custom_oauth.go:133` 附近的冲突标记）。**
- `PUT /:id`（`controller/custom_oauth.go:328`）：请求体 `UpdateCustomOAuthProviderRequest`，多个字段为指针，nil 表示保留原值；`client_secret` 为空串时保留原值（`controller/custom_oauth.go:305`）。

### 关联服务（root）

`api-router:284`，`RootAuth`（`api-router:286`）。

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/linked-service/` | 列出关联服务 | `api-router:288` |
| GET | `/api/linked-service/:id` | 单个关联服务 | `api-router:289` |
| POST | `/api/linked-service/` | 新建关联服务 | `api-router:290` |
| PUT | `/api/linked-service/:id` | 更新 admin_url | `api-router:291` |
| DELETE | `/api/linked-service/:id` | **删除关联服务** | `api-router:292` |
| POST | `/api/linked-service/:id/sync-password` | **把新密码同步到关联服务** | `api-router:293` |

- 新建（`controller/linked_service.go:61`）：请求体 `LinkedServiceCreateReq`（`controller/linked_service.go:17`）：`name`、`adapter`（必填）、`admin_url`。adapter 未知时返回 HTTP 400 `"unknown adapter: <x>"`（`controller/linked_service.go:69`）。响应体不是标准包装：`{"success":true,"data":{...}}`，无 `message`（`controller/linked_service.go:79`）。
- 同步密码（`controller/linked_service.go:131`）：请求体 `LinkedServiceSyncReq`（`controller/linked_service.go:26`）：`admin_password`（必填）。**该密码不落库，仅在 handler 生命周期内驻留内存**（`controller/linked_service.go:26`、`controller/linked_service.go:122`）。

### 平台与 MJ/任务 列表（AdminAuth）

| 方法 | 路径 | 用途 | 行号 |
| --- | --- | --- | --- |
| GET | `/api/mj/` | 全平台 Midjourney 任务（分页） | `api-router:441` |
| GET | `/api/mj/export` | 导出全平台 Midjourney CSV | `api-router:440` |
| GET | `/api/task` | 全平台任务插件任务（分页） | `api-router:448` |
| GET | `/api/task/export` | 导出全平台任务 CSV | `api-router:447` |
| GET | `/api/status/test` | 状态测试（`controller/misc.go:27`） | `api-router:29` |

- `GET /api/mj/`（`controller/midjourney.go:281`）：query `channel_id`、`mj_id`、`start_timestamp`、`end_timestamp`。启用 `MjForwardUrlEnabled` 时图片 URL 会被改写为带签名的本站地址（`controller/midjourney.go:296`）。
- `GET /api/task`（`controller/task.go:375`）：query `platform`、`task_id`、`status`、`action`、`start_timestamp`、`end_timestamp`、`channel_id`。返回分页的 `dto.TaskDto`。

## 本文件未能确定的内容

- `MultiKeyManageRequest` 的逐字段定义（`controller/channel.go` 内声明，仅核对到 `channel_id`、`action`、`page`、`page_size` 被使用）。
- `fetchModelsRequest` 的逐字段定义（`controller/channel.go` 内声明）。
- `tokenGroupMigrationRequest` 的逐字段定义（`controller/token_group.go` 内声明）。
- `ionet.DeploymentRequest`、`ionet.UpdateDeploymentRequest`、`ionet.ExtendDurationRequest`、`ionet.PriceEstimationRequest`、`dto.UpstreamRequest`、`dto.UpstreamDTO`、`dto.SyncableChannel` 的逐字段定义（分属 `ionet` SDK 与 `relaykit/dto`，本次未读取）。
- `DELETE /api/user/:id/bindings/:binding_type` 中 `binding_type` 的合法取值集合（由 `model.User.ClearBinding` 决定，未核对）。
- `POST /api/option/request_policy` 的 `options` map 合法键集合（由 `model.UpdateRequestPolicyOptions` 决定，未核对）。
- 各 `*RateLimit` 中间件的具体阈值与窗口（`middleware/rate-limit.go`）。
- 仓库当前处于未完成的 git 合并中，`controller/custom_oauth.go`、`router/api-router.go`、`common/audio.go`、`Dockerfile`、`controller/misc.go` 含冲突标记。本文件对 `api-router.go` 的描述基于**合并后工作区的当前内容**；其中 `/api/status-check` 在 `api-router:26` 与 `api-router:38` 各注册一次（分别来自合并两侧），Gin 会因重复路由 panic。该项以当前工作区为准，未做取舍。
