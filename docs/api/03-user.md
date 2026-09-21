# 用户自助 API（03）

本文件记录登录用户针对**自己账号**调用的面板接口：账号资料、登录会话、管理访问令牌（PAT）、安全验证、2FA、Passkey、邮箱与第三方账号绑定、API 令牌（relay key）、自己的用量日志与审计、充值与兑换、订阅、邀请与签到、站内支持，以及登录/注册入口。所有路径均以 `/api` 为前缀，注册于 `router/api-router.go`（下称 `api-router`）与 `router/channel-router.go`。

## 通用约定

- 响应包装：成功为 `{"success":true,"message":"","data":...}`（`common/gin.go:232`）；业务失败为 HTTP 200 + `{"success":false,"message":"..."}`（`common/gin.go:218`、`common/gin.go:225`、`common/gin.go:242`）。因此**不能只看 HTTP 状态码判断业务成败**。
- 鉴权失败由中间件返回带 `code` 的非 200 响应，`middleware/auth.go:218`：

  | 状态码 | `code` | 触发条件 |
  | --- | --- | --- |
  | 401 | `AUTH_TOKEN_EXPIRED` | Access Token 过期 |
  | 401 | `AUTH_SESSION_REVOKED` | 登录会话被撤销或不存在 |
  | 401 | `AUTH_UNAUTHORIZED` | 凭据无效 |
  | 401 | `AUTH_USER_DISABLED` | 用户被封禁（`middleware/auth.go:57`） |
  | 401 | `AUTH_USER_INVALID` | 用户名或角色非法（`middleware/auth.go:65`） |
  | 403 | `AUTH_INSUFFICIENT_PRIVILEGE` | 角色不足（`middleware/auth.go:61`） |
  | 500 | `AUTH_INTERNAL_ERROR` | 鉴权链路内部错误 |

- 角色门槛：`UserAuth` = 普通用户（`common.RoleCommonUser`），`AdminAuth` = 管理员，`RootAuth` = root（`middleware/auth.go:100`、`middleware/auth.go:106`、`middleware/auth.go:112`）。
- 分页：`common/page_info.go:65` 解析 `p`（页码）、`page_size`；兼容 `ps`、`size`。`page_size` 上限 `common.MaxPageSize = 100`（`common/page_info.go:11`），`p` 上限 `common.MaxPage = 100000`（`common/page_info.go:14`）。分页响应体为 `PageInfo`：`{"page","page_size","total","items"}`（`common/page_info.go:17`）。
- 需要「安全验证」（Security Proof）的接口会先经 `middleware.RequireSecurityProof` 校验一次一次性验证凭据；未通过时返回 `writeSecurityOperationError` 定义的固定 `code`（`controller/secure_verification.go:38`），例如 `SECURITY_VERIFICATION_FLOW_REQUIRED`、`SECURITY_VERIFICATION_FAILED`、`SECURITY_VERIFICATION_LOCKED`、`SECURITY_PROOF_SCOPE_MISMATCH`。
- 本文件中「用户令牌」指 relay 调用用的 API key（`model/token.go:14` 的 `Token`）；「管理访问令牌」指面板用 PAT（`model.User.AccessToken`，`model/user.go:101`），两者不同，勿混淆。

## 端点总表

### A. 登录、注册与匿名入口（未登录可调用）

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/user/login/encryption-key` | 匿名 | 获取密码登录的加密公钥与 kid（`api-router:88`） |
| POST | `/api/user/login` | 匿名 | 密码登录（`api-router:89`） |
| POST | `/api/user/login/verify` | 匿名 | 完成登录验证流程（2FA）（`api-router:91`） |
| POST | `/api/user/login/2fa` | 匿名 | 同 `/login/verify`（`api-router:90`，`controller/twofa.go:212`） |
| POST | `/api/user/login/passkey/begin` | 匿名 | Passkey 登录开始（`api-router:92`） |
| POST | `/api/user/login/passkey/finish` | 匿名 | Passkey 登录完成（`api-router:93`） |
| POST | `/api/user/passkey/login/begin` | 匿名 | Passkey 登录开始（另一路径，`api-router:94`） |
| POST | `/api/user/passkey/login/finish` | 匿名 | Passkey 登录完成（另一路径，`api-router:95`） |
| POST | `/api/user/register` | 匿名 | 密码注册（`api-router:82`） |
| GET | `/api/user/register_ticket` | 匿名 | 领取一次性注册票据（`api-router:83`） |
| POST | `/api/user/auth/refresh` | Refresh Cookie | 轮换 Access Token（`api-router:79`） |
| POST | `/api/user/auth/logout` | Cookie/Bearer | 注销当前登录会话（`api-router:81`） |
| GET | `/api/user/groups` | 匿名 | 当前请求可见的可用分组（`api-router:99`） |
| GET | `/api/verification` | 匿名（限流+验证码） | 发送邮箱验证码（`api-router:47`） |
| GET | `/api/reset_password` | 匿名（限流+验证码） | 发送密码重置邮件（`api-router:48`） |
| POST | `/api/user/reset` | 匿名（限流） | 凭重置票据重置密码（`api-router:49`） |
| POST | `/api/oauth/state` | 可选登录 | 创建 OAuth 授权流程 state（`api-router:51`） |
| GET | `/api/oauth/:provider` | 可选登录 | 标准 OAuth 回调（`api-router:62`） |
| GET | `/api/oauth/wechat` | 匿名（限流） | 微信登录（`api-router:56`） |
| GET | `/api/oauth/telegram/login` | 匿名（限流） | Telegram 登录（`api-router:58`） |

### B. 账号资料

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/user/self` | UserAuth | 读取自己的账号与权限（`api-router:108`） |
| PUT | `/api/user/self` | UserAuth | 改用户名/显示名/密码，或更新边栏、语言（`api-router:110`） |
| DELETE | `/api/user/self` | UserAuth + 安全验证 | 删除自己的账号（`api-router:111`） |
| GET | `/api/user/models` | UserAuth | 自己可用的模型名列表（`api-router:109`） |
| GET | `/api/user/self/groups` | UserAuth | 自己的可用分组及倍率（`api-router:107`） |
| PUT | `/api/user/setting` | UserAuth | 更新额度预警、通知等个人设置（`api-router:146`） |
| GET | `/api/user/aff` | UserAuth | 读取/生成自己的邀请码（`api-router:122`） |
| POST | `/api/user/aff_transfer` | UserAuth（用户级限流） | 邀请额度转入余额（`api-router:145`） |
| GET | `/api/user/checkin` | UserAuth | 签到状态与本月统计（`api-router:157`） |
| POST | `/api/user/checkin` | UserAuth（验证码） | 执行签到（`api-router:158`） |

### C. 登录会话

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/user/sessions` | UserAuth + 浏览器会话 | 列出自己的登录会话（`api-router:104`） |
| DELETE | `/api/user/sessions/:sid` | UserAuth + 浏览器会话 | 撤销指定会话（`api-router:105`） |
| POST | `/api/user/sessions/revoke-others` | UserAuth + 浏览器会话 | 撤销除当前外的全部会话（`api-router:106`） |

### D. 管理访问令牌（PAT）

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/user/token/status` | UserAuth | 查询是否已存在 PAT（`api-router:113`） |
| GET | `/api/user/token` | UserAuth + 安全验证 + 限流 | 生成并返回新 PAT（`api-router:112`） |
| POST | `/api/user/token` | UserAuth + 安全验证 + 限流 | 同 GET（`api-router:114`） |
| DELETE | `/api/user/token` | UserAuth + 安全验证 + 限流 | 撤销 PAT（`api-router:115`） |

### E. 安全验证（Security Proof）

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/verify/methods` | UserAuth | 查询某 scope 下可用的验证方式（`api-router:74`） |
| POST | `/api/verify` | UserAuth + 限流 | 提交验证凭据换取一次性授权（`api-router:75`） |

### F. 两步验证（2FA / TOTP）

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/user/2fa/status` | UserAuth | 2FA 启用与锁定状态、备用码剩余数（`api-router:149`） |
| POST | `/api/user/2fa/setup` | UserAuth + 安全验证 | 开始 2FA 绑定，返回密钥（`api-router:150`） |
| POST | `/api/user/2fa/enable` | UserAuth + 浏览器会话 | 校验 TOTP 后启用 2FA（`api-router:151`） |
| POST | `/api/user/2fa/disable` | UserAuth + 安全验证 | 关闭 2FA（`api-router:152`） |
| PUT | `/api/user/2fa/login-verification` | UserAuth + 浏览器会话 | 开关「登录时要求 2FA」（`api-router:153`） |
| POST | `/api/user/2fa/backup_codes` | UserAuth + 安全验证 | 重新生成备用码（`api-router:154`） |

### G. Passkey

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/user/passkey` | UserAuth | 查询是否已绑定 Passkey（`api-router:116`） |
| POST | `/api/user/passkey/register/begin` | UserAuth + 安全验证 | 开始注册 Passkey（`api-router:117`） |
| POST | `/api/user/passkey/register/finish` | UserAuth | 完成注册 Passkey（`api-router:118`） |
| POST | `/api/user/passkey/verify/begin` | UserAuth | 开始 Passkey 步进验证（`api-router:119`） |
| POST | `/api/user/passkey/verify/finish` | UserAuth | 完成 Passkey 步进验证（`api-router:120`） |
| DELETE | `/api/user/passkey` | UserAuth + 安全验证 | 解绑 Passkey（`api-router:121`） |

### H. 邮箱与第三方账号绑定

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| POST | `/api/oauth/email/bind/start` | UserAuth + 安全验证 + 限流 | 发起邮箱绑定（`api-router:52`） |
| POST | `/api/oauth/email/bind/resend` | UserAuth + 限流 | 重发邮箱绑定验证码（`api-router:53`） |
| POST | `/api/oauth/email/bind` | UserAuth + 限流 | 提交验证码完成邮箱绑定（`api-router:54`） |
| POST | `/api/oauth/wechat/bind` | UserAuth + 限流 | 绑定微信（`api-router:57`） |
| POST | `/api/oauth/telegram/bind/start` | UserAuth + 限流 | 发起 Telegram 绑定（`api-router:59`） |
| GET | `/api/oauth/telegram/bind/:flow_token` | UserAuth + 限流 | Telegram 绑定回调（`api-router:60`） |
| GET | `/api/user/oauth/bindings` | UserAuth | 列出自己的 OAuth 绑定（`api-router:161`） |
| DELETE | `/api/user/oauth/bindings/:provider_id` | UserAuth + 安全验证 | 解绑指定自定义 OAuth 提供方（`api-router:162`） |

### I. API 令牌（relay key）

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/token/` | UserAuth | 分页列出自己的令牌（`api-router:341`） |
| GET | `/api/token/search` | UserAuth（搜索限流） | 按关键字搜索自己的令牌（`api-router:343`） |
| GET | `/api/token/rpm` | UserAuth | 批量查询令牌 RPM（`api-router:342`） |
| GET | `/api/token/auto-groups` | UserAuth | auto 分组的可选分组与上限（`api-router:344`） |
| GET | `/api/token/:id` | UserAuth | 读取单个令牌（脱敏）（`api-router:347`） |
| GET | `/api/token/:id/auto-routes` | UserAuth | 读取令牌的虚拟模型路由（`api-router:345`） |
| GET | `/api/token/:id/auto-routes/status` | UserAuth | 虚拟模型路由的可用性快照（`api-router:346`） |
| POST | `/api/token/` | UserAuth | 新建令牌（`api-router:350`） |
| PUT | `/api/token/` | UserAuth | 更新令牌（`api-router:351`） |
| DELETE | `/api/token/:id` | UserAuth | 删除令牌（`api-router:352`） |
| POST | `/api/token/batch` | UserAuth | 批量删除令牌（`api-router:353`） |
| POST | `/api/token/:id/key` | UserAuth + 限流 | 查看令牌完整 key（`api-router:348`） |
| POST | `/api/token/:id/reset-used-quota` | UserAuth | 清零令牌已用额度（`api-router:349`） |
| POST | `/api/token/batch/keys` | UserAuth + 限流 | 批量查看完整 key（最多 100 个，`api-router:354`） |
| GET | `/api/usage/token/` | TokenAuthReadOnly | 用令牌自身查询其额度摘要（`api-router:363`） |

### J. 自己的用量、日志与审计

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/log/self` | UserAuth | 自己的调用日志（`api-router:390`） |
| GET | `/api/log/self/stat` | UserAuth | 自己的日志统计（`api-router:387`） |
| GET | `/api/log/self/export` | UserAuth | 导出自己的日志 CSV（`api-router:391`） |
| GET | `/api/log/self/search` | UserAuth（搜索限流） | 已废弃，固定返回失败（`api-router:392`，`controller/log.go:124`） |
| GET | `/api/log/token` | TokenAuthReadOnly | 用令牌查询其日志（`api-router:420`） |
| GET | `/api/data/self` | UserAuth | 自己的额度消耗按日聚合（`api-router:414`） |
| GET | `/api/data/flow/self` | UserAuth | 自己的流量额度按日聚合（`api-router:416`） |
| GET | `/api/audit/self` | UserAuth | 自己的审计日志（`api-router:381`） |
| GET | `/api/audit/self/export` | UserAuth | 导出自己的审计日志 CSV（`api-router:382`） |
| GET | `/api/mj/self` | UserAuth | 自己的 Midjourney 任务（`api-router:438`） |
| GET | `/api/mj/self/export` | UserAuth | 导出自己的 Midjourney CSV（`api-router:439`） |
| GET | `/api/task/self` | UserAuth | 自己的任务插件任务（`api-router:445`） |
| GET | `/api/task/self/export` | UserAuth | 导出自己的任务 CSV（`api-router:446`） |
| GET | `/api/task/:task_id/artifacts` | UserAuth | 读取任务产物清单（`api-router:449`） |

### K. 充值、兑换与订阅

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/user/topup/info` | UserAuth | 充值方式、限额与合规状态（`api-router:123`） |
| GET | `/api/user/topup/self` | UserAuth | 自己的充值记录（`api-router:124`） |
| POST | `/api/user/topup` | UserAuth（用户级限流+验证码） | 兑换码充值（`api-router:131`） |
| POST | `/api/user/amount` | UserAuth | 估算易支付应付金额（`api-router:133`） |
| POST | `/api/user/pay` | UserAuth + 限流 | 创建易支付订单（`api-router:132`） |
| POST | `/api/user/stripe/amount` | UserAuth | 估算 Stripe 应付金额（`api-router:135`） |
| POST | `/api/user/stripe/pay` | UserAuth + 限流 | 创建 Stripe 订单（`api-router:134`） |
| POST | `/api/user/creem/pay` | UserAuth + 限流 | 创建 Creem 订单（`api-router:136`） |
| POST | `/api/user/waffo/amount` | UserAuth | 估算 Waffo 应付金额（`api-router:137`） |
| POST | `/api/user/waffo/pay` | UserAuth + 限流 | 创建 Waffo 订单（`api-router:138`） |
| POST | `/api/user/waffo-pancake/amount` | UserAuth | 估算 Waffo Pancake 应付金额（`api-router:139`） |
| POST | `/api/user/waffo-pancake/pay` | UserAuth + 限流 | 创建 Waffo Pancake 订单（`api-router:140`） |
| POST | `/api/user/monero/pay` | UserAuth + 限流 | 创建 Monero 子地址发票（`api-router:141`） |
| GET | `/api/user/monero/payment` | UserAuth | 查询 Monero 发票支付状态（`api-router:142`） |
| POST | `/api/user/nowpayments/pay` | UserAuth + 限流 | 创建 NOWPayments 发票（`api-router:143`） |
| GET | `/api/user/nowpayments/payment` | UserAuth | 查询 NOWPayments 支付状态（`api-router:144`） |
| GET | `/api/user/redemption/self` | UserAuth | 自己购买的兑换码（`api-router:125`） |
| POST | `/api/user/redemption/purchase/amount` | UserAuth + 限流 | 估算购买兑换码应付金额（`api-router:126`） |
| POST | `/api/user/redemption/purchase` | UserAuth + 限流 | 购买兑换码（`api-router:127`） |
| POST | `/api/user/redemption/:id/refund` | UserAuth（用户级限流） | 退款自己购买的兑换码（`api-router:128`） |
| GET | `/api/subscription/plans` | UserAuth | 可用订阅套餐（`api-router:193`） |
| GET | `/api/subscription/self` | UserAuth | 自己的订阅与计费偏好（`api-router:194`） |
| PUT | `/api/subscription/self/preference` | UserAuth | 修改计费偏好（`api-router:195`） |
| POST | `/api/subscription/balance/pay` | UserAuth + 限流 | 用余额购买订阅（`api-router:196`） |
| POST | `/api/subscription/epay/pay` | UserAuth + 限流 | 易支付购买订阅（`api-router:197`） |
| POST | `/api/subscription/stripe/pay` | UserAuth + 限流 | Stripe 购买订阅（`api-router:198`） |
| POST | `/api/subscription/creem/pay` | UserAuth + 限流 | Creem 购买订阅（`api-router:199`） |
| POST | `/api/subscription/waffo-pancake/pay` | UserAuth + 限流 | Waffo Pancake 购买订阅（`api-router:200`） |
| POST | `/api/subscription/nowpayments/pay` | UserAuth + 限流 | NOWPayments 购买订阅（`api-router:201`） |

### L. 站内支持

| 方法 | 路径 | 角色/权限 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/support/unread` | UserAuth + SupportEnabled | 未读消息数（`api-router:227`） |
| GET | `/api/support/conversation` | UserAuth + SupportEnabled | 自己的支持会话与消息（`api-router:228`） |
| GET | `/api/support/orders` | UserAuth + SupportEnabled | 可引用的订单列表（`api-router:229`） |
| POST | `/api/support/messages` | UserAuth + SupportEnabled（上传限流） | 发送消息/图片（`api-router:230`） |

`middleware.SupportEnabled` 在站内信未启用时返回 HTTP 404 + `{"success":false,"message":"站内信功能未启用"}`（`middleware/support.go:25`）。

## 关键端点详述

### GET /api/user/self

`controller/user.go:496`。无请求参数。

响应 `data` 为 `buildSelfUserData`（`controller/user.go:523`）的白名单字段：`id`、`username`、`display_name`、`has_password`、`role`、`status`、`email`、`github_id`、`github_created_at`、`discord_id`、`oidc_id`、`wechat_id`、`telegram_id`、`group`、`quota`、`used_quota`、`request_count`、`aff_code`、`aff_count`、`aff_quota`、`aff_history_quota`、`inviter_id`、`linux_do_id`、`setting`、`stripe_customer`、`sidebar_modules`、`permissions`。该 DTO 有意排除密码、PAT 与管理员备注（`controller/user.go:520`）。`permissions` 含 `sidebar_settings`、`sidebar_modules` 与 `admin_permissions`（后者是 `authz.Capabilities` 的完整资源/动作矩阵，`controller/user.go:509`、`service/authz/resolver.go:38`）。

### PUT /api/user/self

`controller/user.go:795`。请求体是一个自由 JSON 对象，按字段分三条互斥路径：

1. 含 `sidebar_modules`（字符串）且未请求改密码：仅更新该字段，返回 `common.ApiSuccessI18n`（`controller/user.go:814`）。
2. 含 `language`（字符串）且未请求改密码：仅更新语言偏好（`controller/user.go:840`）。
3. 其余：按 `model.User` 解析，接受 `username`、`display_name`、`password`、`original_password`。

路径 3 中若 `password` 非空，则要求存在浏览器会话并通过安全验证（scope 为 `password_change`，首次设密码为 `password_set`，`controller/user.go:900`）；成功后返回轮换后的 `access_token`、`token_type`、`access_expires_at`、`session`、`has_password`、`notification_warning`（`controller/user.go:926`）。安全策略拒绝时返回 `PASSWORD_POLICY_REJECTED`、`CURRENT_PASSWORD_INVALID`、`ACCOUNT_SECURITY_STATE_CHANGED`（`controller/secure_verification.go:60`、`controller/secure_verification.go:62`、`controller/secure_verification.go:64`）。

### DELETE /api/user/self

`controller/user.go:981`。需通过 scope 为 `account_delete` 的安全验证；root 用户被拒绝（`MsgUserCannotDeleteRootUser`，`controller/user.go:993`）。成功时清除 Refresh Cookie 并返回空 `data`。

### POST /api/user/login

`controller/user.go:55`。请求体 `LoginRequest`（`controller/user.go:31`）：`username`、`password`、`password_encrypted`、`encryption_key_id`。当服务端启用密码登录加密时（`common.PasswordLoginEncryptionEnabled`），必须传 `password_encrypted` + `encryption_key_id`，明文 `password` 被忽略（`controller/user.go:68`）。公钥与 kid 由 `GET /api/user/login/encryption-key` 提供（`controller/user.go:38`）。

成功响应与 `writeLoginResponse` 一致（`controller/user.go:228`）：`data.access_token`、`token_type`、`access_expires_at`、`session`、`user`；同时写入 Refresh Cookie。若登录策略要求二次验证，则返回 `common.ApiSuccess(challenge)` 而非会话（`controller/user.go:175`），客户端需转 `/api/user/login/verify`。被封禁账号返回 HTTP 200 + `{"success":false,"message":...,"data":{"banned":true,"html":"..."}}`（`controller/user.go:109`）。

### POST /api/user/auth/refresh

`controller/auth_session.go:17`。请求体为空，凭 `HttpOnly` Refresh Cookie（`service.RefreshCookieName`）与 `X-Auth-Session` 头。返回 `access_token`、`token_type`、`access_expires_at`、`user`、`session`（`controller/auth_session.go:34`），并轮换 Refresh Cookie。失败时返回 `writeAuthSessionError` 的 `code`（`controller/auth_session.go:164`）。

### POST /api/user/auth/logout

`controller/auth_session.go:47`。优先用 `Authorization: Bearer` 中的 Access Token 定位并撤销会话；否则回退到 Refresh Cookie。返回 `data.revoked_sid`、`data.cookie_cleared`，或仅 `{"success":true}`（`controller/auth_session.go:76`、`controller/auth_session.go:86`）。SID 不匹配时返回 `ErrLoginSessionMismatch` 对应的错误码（`controller/auth_session.go:53`）。

### GET /api/user/sessions

`controller/auth_session.go:97`。要求浏览器会话（PAT 调用会被拒，`controller/auth_session.go:151` 返回 HTTP 403 `AUTH_SESSION_REQUIRED`）。返回 `data` 为会话数组（`service.ListLoginSessions`）。

### DELETE /api/user/sessions/:sid

`controller/auth_session.go:110`。`sid` 为空 → HTTP 400 `AUTH_SESSION_ID_REQUIRED`；不存在或不属于本人 → HTTP 404 `AUTH_SESSION_NOT_FOUND`；成功返回 `data.revoked_sid`、`data.current`（`controller/auth_session.go:117`、`controller/auth_session.go:126`、`controller/auth_session.go:135`）。

### POST /api/user/sessions/revoke-others

`controller/auth_session.go:138`。返回 `data.revoked_count`。

### GET|POST|DELETE /api/user/token（管理访问令牌）

- `GET /api/user/token/status`（`controller/access_token.go:17`）：返回 `model.GetUserAccessTokenStatus`。
- `GET`/`POST /api/user/token`（`controller/access_token.go:26`）：先经 `RequireSecurityProof`（scope `access_token_generate`），生成 29–32 字符随机 key，写入 `user.access_token`，并记录 `access_token.generate` 审计（仅记录指纹）。成功返回 `data` 为完整 key 字符串。
- `DELETE /api/user/token`（`controller/access_token.go:53`）：scope `access_token_revoke`，撤销后返回 `data: null`。

**注意**：这三个接口返回/撤销的是面板管理令牌，具备该用户的面板权限，属于凭据操作。

### GET /api/verify/methods 与 POST /api/verify

- `GET /api/verify/methods?scope=<scope>`（`controller/secure_verification.go:22`）：要求浏览器会话，返回该 scope 下 `service.GetVerificationRequirements` 的结果。非会话认证返回 HTTP 401（`controller/secure_verification.go:24`）。
- `POST /api/verify`（`controller/secure_verification.go:137`）：请求体为 `service.VerificationInput`（字段定义在 `service/` 内，本文件未逐一核对，**未能从 controller 层确定字段名**）。成功返回 `proof`（含 `Method`、`Scope`），并记录 `user.security_verify` 审计。

### GET /api/user/2fa/status

`controller/twofa.go:94`。返回 `data.enabled`、`data.locked`；启用时附带 `data.backup_codes_remaining`。

### POST /api/user/2fa/setup

`controller/twofa.go:27`。需安全验证（scope `twofa_setup`）与浏览器会话，返回 `service.StartTwoFASetup` 的结果（通常含 TOTP 密钥与 otpauth URL），并记录 `user.2fa_setup` 审计。**具体字段未在本文件核对**。

### POST /api/user/2fa/enable

`controller/twofa.go:42`。请求体 `Verify2FARequest`（`controller/twofa.go:18`）：`code`（必填）、`flow_token`。成功后把当前会话推进到新的用户鉴权版本，返回 `authRotationData`（`controller/auth_session.go:182`）：`access_token`、`token_type`、`access_expires_at`、`session`。

### POST /api/user/2fa/disable

`controller/twofa.go:67`。需安全验证（scope `twofa_disable`）；返回 `authRotationData` + 中文提示。

### PUT /api/user/2fa/login-verification

`controller/twofa.go:129`。请求体 `{"enabled": <bool>}`；关闭时需安全验证（scope `login_twofa_disable`）。关闭时同时返回轮换后的会话字段（`controller/twofa.go:156`）。

### POST /api/user/2fa/backup_codes

`controller/twofa.go:167`。需安全验证（scope `twofa_backup_codes`）。返回 `data.backup_codes`（明文数组，仅此一次）+ `authRotationData`。**该响应含凭据，需谨慎处理。**

### Passkey 流程

- `GET /api/user/passkey`（`controller/passkey.go:250`）：返回 `data.enabled`，已绑定时含 `data.last_used_at`。
- `POST /api/user/passkey/register/begin`（`controller/passkey.go:45`）：管理员未启用 Passkey 时返回 `{"success":false,"message":"管理员未启用 Passkey 登录"}`；需安全验证（scope `passkey_register`）。返回 `data.options`（WebAuthn 注册选项）、`data.flow_token`、`data.expires_at`。
- `POST /api/user/passkey/register/finish`（`controller/passkey.go:121`）：请求体 `{flow_token, credential}`（`controller/passkey.go:20`）。
- `POST /api/user/passkey/verify/begin`（`controller/passkey.go:492`）：请求体 `{rp_id, scope, context?}`（`controller/passkey.go:26`），返回 `data.options`、`data.rp_ids`、`data.flow_token`、`data.expires_at`。
- `POST /api/user/passkey/verify/finish`（`controller/passkey.go:574`）：请求体同 finish 形式。
- `DELETE /api/user/passkey`（`controller/passkey.go:216`）：需安全验证（scope `passkey_delete`），成功后返回 `authRotationData` + 提示。

### 邮箱绑定

- `POST /api/oauth/email/bind/start`（`controller/email_binding.go:20`）：请求体 `emailBindRequest`（`controller/email_binding.go:12`）：`email`、`flow_token`、`new_code`、`old_code`；本接口只用 `email`。需安全验证（scope `account_bind`，context 含 provider=email 与目标邮箱）。邮箱被占用返回 `EMAIL_ALREADY_TAKEN`（`controller/secure_verification.go:49`）；地址被拒返回 `EMAIL_ADDRESS_REJECTED`。
- `POST /api/oauth/email/bind/resend`（`controller/email_binding.go:58`）：需 `flow_token`。重发间隔未到返回 HTTP 429 `EMAIL_BINDING_RESEND_WAIT`（`controller/secure_verification.go:57`）。
- `POST /api/oauth/email/bind`（`controller/email_binding.go:82`）：提交 `flow_token` 与验证码；错误码见 `EMAIL_BINDING_CODE_INVALID`、`EMAIL_BINDING_LOCKED`（`controller/secure_verification.go:53`、`controller/secure_verification.go:55`）。

### 令牌管理（`/api/token/`）

- `GET /api/token/`（`controller/token.go:218`）：分页返回**脱敏**令牌。响应 `items` 元素为 `tokenResponse`（`controller/token.go:56`）：内嵌 `model.Token`（`key` 字段被替换为掩码，`controller/token.go:81`），外加 `auto_groups`、`auto_routes`（列表响应中省略，`controller/token.go:102`）、`total_quota`、`rpm`。
- `GET /api/token/rpm?ids=1,2`（`controller/token.go:269`）：`ids` 为可重复 query 参数，逗号分隔亦可；最多 100 个（`controller/token.go:64`、`controller/token.go:301`）。返回 `data.rpms`（token id → rpm）。
- `GET /api/token/auto-groups`（`controller/token.go:449`）：返回 `data.groups`、`data.max_count`。
- `GET /api/token/:id/auto-routes`（`controller/token.go:327`）：返回 `data.auto_routes`。
- `GET /api/token/:id/auto-routes/status`（`controller/token.go:355`）：令牌分组非 `auto` 时返回失败「虚拟模型路由仅支持 auto 分组」（`controller/token.go:367`）。成功返回 `data.routes`（元素含 `virtual_model`、`chain`、`models`）、`data.auto_groups`、`data.updated_at`。
- `POST /api/token/`（`controller/token.go:548`）：请求体 `tokenRequest`（`controller/token.go:50`）= `model.Token` 字段 + `auto_groups`（数组或 null）+ `auto_routes`（对象或 null）。约束：`name` 长度 ≤ 50（`controller/token.go:559`）；`remain_quota` 不得为负且不得超过上限（`controller/token.go:565`）；令牌总数不得超过 `operation_setting.GetMaxUserTokens()`（`controller/token.go:576`）；`expired_time` 被服务端强制为 `-1`（永不过期，`controller/token.go:558`）。仅当 `group == "auto"` 时才接受 `auto_groups`/`auto_routes`，否则两者被清空且 `cross_group_retry` 置 false（`controller/token.go:589`）。成功返回 `common.ApiSuccess(nil)`（`controller/token.go:632`，**返回体不含 key，需另调 `POST /api/token/:id/key`**）。
- `PUT /api/token/`（`controller/token.go:649`）：`?status_only=<非空>` 时只更新 `status`（`controller/token.go:680`）。启用令牌时若已过期或已耗尽会被拒绝（`MsgTokenExpiredCannotEnable`、`MsgTokenExhaustedCannotEable`，`controller/token.go:682`）。成功返回脱敏后的 `tokenResponse`。
- `DELETE /api/token/:id`（`controller/token.go:635`）：仅能删除本人令牌（`model.DeleteTokenById(id, userId)`）。
- `POST /api/token/batch`（`controller/token.go:736`）：请求体 `{"ids":[...]}`，返回 `data` 为删除条数。
- `POST /api/token/:id/key`（`controller/token.go:461`）：返回 `data.key` 为**完整明文 key**（凭据操作，受 `CriticalRateLimit` + `DisableCache`）。
- `POST /api/token/:id/reset-used-quota`（`controller/token.go:435`）：返回更新后的脱敏令牌。
- `POST /api/token/batch/keys`（`controller/token.go:755`）：请求体 `{"ids":[...]}`，最多 100 个（`controller/token.go:761`），返回 `data.keys`（id → 完整 key）。

### GET /api/usage/token/

`controller/token.go:499`。中间件为 `TokenAuthReadOnly`（`middleware/auth.go:288`），即仅校验 key 存在、不校验状态/过期/额度，但拒绝已禁用令牌（`middleware/auth.go:326`）。响应不是标准包装：直接返回 `{"code":true,"message":"ok","data":{...}}`，`data` 含 `object`（`"token_usage"`）、`name`、`total_granted`、`total_used`、`total_available`、`unlimited_quota`、`model_limits`、`model_limits_enabled`、`expires_at`（`controller/token.go:531`）。

### GET /api/log/self

`controller/log.go:45`。Query 参数：`type`（整数）、`start_timestamp`、`end_timestamp`、`token_name`、`model_name`、`group`、`request_id`、`upstream_request_id`，以及分页参数。返回分页的日志数组。

### GET /api/log/self/stat

`controller/log.go:182`。Query 参数同上去掉 `request_id`；`username` 取自当前登录态（忽略客户端传入）。返回 `data.quota`、`data.rpm`、`data.tpm`（`controller/log.go:197`）。

### GET /api/log/self/export

`controller/log.go:67`。`selfView` 判定为 `c.FullPath() == "/api/log/self/export"`（`controller/log.go:74`），此时强制 `userId = 当前用户`、`username = ""`、`channel = 0`。响应为 `text/csv` 附件下载，文件名 `usage-logs-<时间戳>.csv`（`controller/log.go:85`）。

### GET /api/data/self 与 GET /api/data/flow/self

- `controller/usedata.go:63`：`start_timestamp`、`end_timestamp` 为可选 query。时间跨度超过 2592000 秒（30 天）时返回失败「时间跨度不能超过 1 个月」（`controller/usedata.go:68`）。
- `controller/usedata.go:107`：`start_timestamp`、`end_timestamp` **必填且必须为正数**，否则返回「invalid start_timestamp」/「invalid end_timestamp」/「invalid time range」（`controller/usedata.go:13`）；同样受 30 天跨度限制。

### GET /api/audit/self 与 GET /api/audit/self/export

`controller/access_token.go:68`、`controller/access_token.go:128`。`selfView` 由 `c.FullPath() == "/api/audit/self"`（或 `/api/audit/self/export`）判定，判定为真时忽略客户端的 `username` 参数并强制按当前用户过滤（`controller/access_token.go:99`）。Query 参数：`username`、`category`、`token_ref`、`exclude_token_ref`、`request_id`、`start_timestamp`、`end_timestamp`、`success`（`"true"`/`"false"`）。

校验规则（`controller/access_token.go:103`）：`category` 必须是空或 `login`/`security`/`operation`/`access_token` 之一（`model/audit_log.go:455`）；`token_ref`/`exclude_token_ref` 必须是 64 位小写十六进制（`model/audit_log.go:459`）；时间戳必须为非负整数且 `end >= start`；`success` 只能是 `true`/`false`。违反任一规则返回 HTTP 200 + `{"success":false,"message":"Invalid audit filters"}` 等。

### 充值入口

- `GET /api/user/topup/info`（`controller/topup.go:29`）：返回可用支付方式与限额。`data` 字段（`controller/topup.go:153`）：`enable_online_topup`、`enable_stripe_topup`、`enable_creem_topup`、`enable_waffo_topup`、`enable_waffo_pancake_topup`、`enable_monero_topup`、`enable_nowpayments_topup`、`nowpayments_min_topup`、`nowpayments_pay_currencies`、`enable_redemption`、`enable_redemption_purchase`、`payment_compliance_confirmed`、`payment_compliance_terms_version`、`waffo_pay_methods`、`creem_products`、`pay_methods`、`min_topup`、`stripe_min_topup`、`waffo_min_topup`、`waffo_pancake_min_topup`、`amount_options`、`discount`、`redemption_purchase_methods`、`topup_link`、`payment_announcement`。
- `POST /api/user/topup`（`controller/user.go:1361`）：请求体 `{"key":"<兑换码>"}`（`controller/user.go:1316`）。未确认支付合规时返回失败（`controller/user.go:1362`）。成功返回 `data` 为额度数值，或订阅型兑换码时为 `{"subscription_plan_id","subscription_plan_title"}`（`controller/user.go:1387`）。兑换失败**不暴露具体原因**（`controller/user.go:1382`）。
- `POST /api/user/amount`（`controller/topup.go:696`）：请求体 `AmountRequest`（`controller/topup.go:210`）：`amount`、`payment_method`、`epay_gateway`。响应格式特殊：`{"message":"success","data":"<应付金额字符串>"}` 或 `{"message":"error","data":"<原因>"}`（`controller/topup.go:726`）。
- `POST /api/user/pay`（`controller/topup.go:462`）：请求体 `EpayRequest`（同 `AmountRequest` 结构）。成功返回 `{"message":"success","data":<表单参数>,"url":"<支付网关地址>"}`（`controller/topup.go:552`）。
- `POST /api/user/stripe/pay` / `POST /api/user/stripe/amount`（`controller/topup_stripe.go:155`、`controller/topup_stripe.go:145`）：请求体 `StripePayRequest`（定义在 `controller/topup_stripe.go` 内，本文件未逐字段核对）。`pay` 成功返回含 `pay_link` 的数据（`controller/topup_stripe.go:140`）。
- `POST /api/user/creem/pay`（`controller/topup_creem.go:152`）：请求体 `CreemPayRequest`；成功返回 `{"checkout_url","order_id"}`（`controller/topup_creem.go:146`）。
- `POST /api/user/waffo/amount`（`controller/topup_waffo.go:114`）与 `POST /api/user/waffo/pay`（`controller/topup_waffo.go:147`）：请求体 `WaffoPayRequest`，含 `amount`、`pay_method_index`（新协议）与已废弃的 `pay_method_type`/`pay_method_name`。`amount` 小于 `setting.WaffoMinTopUp` 时失败（`controller/topup_waffo.go:151`）。`pay_method_index` 越界或旧字段不在服务端列表中时返回「不支持的支付方式」（`controller/topup_waffo.go:167`、`controller/topup_waffo.go:180`）。
- `POST /api/user/waffo-pancake/amount` / `pay`（`controller/topup_waffo_pancake.go:82`、`controller/topup_waffo_pancake.go:463`）。
- `POST /api/user/monero/pay`（`controller/topup_monero.go:19`）：请求体 `{"amount":<int64>}`，返回 `service.CreateMoneroInvoice` 的发票对象。`GET /api/user/monero/payment?address=<地址>`（`controller/topup_monero.go:33`）：`address` 必填，否则返回失败。
- `POST /api/user/nowpayments/pay`（`controller/topup_nowpayments.go:20`）：请求体 `{"amount","pay_currency"}`，返回发票对象。`GET /api/user/nowpayments/payment?id=<payment id>`（`controller/topup_nowpayments.go:34`）：`id` 必填。

### 兑换码购买与退款

- `GET /api/user/redemption/self`（`controller/user_redemption.go:13`）：分页返回本人购买/持有的兑换码。
- `POST /api/user/redemption/purchase/amount`（`controller/redemption_purchase.go:252`）：请求体 `RedemptionPurchaseRequest`（定义在 `controller/redemption_purchase.go` 内，**字段未逐字段核对**）。响应格式为 `{"message":"success","data":"<金额>"}`；Monero 时 `data` 为空字符串（`controller/redemption_purchase.go:262`）。
- `POST /api/user/redemption/purchase`（`controller/redemption_purchase.go:270`）：按 `payment_method` 分派到 Stripe/Waffo/Waffo Pancake/Monero/NOWPayments 分支；Monero 与 NOWPayments 返回发票对象（`controller/redemption_purchase.go:284`、`controller/redemption_purchase.go:291`）。
- `POST /api/user/redemption/:id/refund`（`controller/user_redemption.go:25`）：仅能退自己的兑换码，不属于本人时返回「兑换码不属于当前用户」（`controller/user_redemption.go:34`）。成功返回 `data.quota`。**破坏性操作（退款）。**

### 订阅

- `GET /api/subscription/plans`（`controller/subscription.go:94`）：未确认支付合规时返回空数组（`controller/subscription.go:95`）。否则返回启用中的套餐，元素为 `{"plan": {...}}`（`controller/subscription.go:18`）。
- `GET /api/subscription/self`（`controller/subscription.go:115`）：返回 `data.billing_preference`、`data.subscriptions`（仅生效中）、`data.all_subscriptions`（含已过期）。
- `PUT /api/subscription/self/preference`（`controller/subscription.go:139`）：请求体 `{"billing_preference":"..."}`，经 `common.NormalizeBillingPreference` 归一化。返回归一化后的值。
- `POST /api/subscription/balance/pay`（`controller/subscription.go:162`）：请求体 `{"plan_id":<int>}`，用余额购买，返回 `data: null`。
- `POST /api/subscription/epay/pay`（`controller/subscription_payment_epay.go:26`）：请求体 `SubscriptionEpayPayRequest`（含 `plan_id`、`payment_method`、`epay_gateway`）。校验套餐启用、金额 ≥ 0.01、购买上限（`controller/subscription_payment_epay.go:46`、`controller/subscription_payment_epay.go:58`）。成功返回 `{"message":"success","data":<表单参数>,"url":<网关地址>}`。
- `POST /api/subscription/stripe/pay`（`controller/subscription_payment_stripe.go:23`）：套餐未配置 `stripe_price_id` 时失败（`controller/subscription_payment_stripe.go:43`）。
- `POST /api/subscription/creem/pay`（`controller/subscription_payment_creem.go:23`）。
- `POST /api/subscription/waffo-pancake/pay`（`controller/subscription_payment_waffo_pancake.go:22`）：套餐未配置 `waffo_pancake_product_id` 时失败。
- `POST /api/subscription/nowpayments/pay`（`controller/subscription_payment_nowpayments.go:14`）：请求体 `SubscriptionNowPaymentsPayRequest`（含 `plan_id`、`pay_currency`），返回发票对象。

### 站内支持

- `GET /api/support/unread`（`controller/support.go:161`）：返回 `data.count`。
- `GET /api/support/conversation`（`controller/support.go:138`）：返回 `{conversation, messages}`（`controller/support.go:26`）。读取会将该会话对当前角色标记为已读（`controller/support.go:120`）。
- `GET /api/support/orders`（`controller/support.go:170`）：返回可引用为工单的订单列表。
- `POST /api/support/messages`（`controller/support.go:204`）：支持 `application/json` 或 `multipart/form-data`（`controller/support.go:34`）。字段：`kind`、`content`、`order_type`、`order_id`、`image_data`（base64）、`grant_quota`、`grant_plan_id`（`controller/support.go:16`）；multipart 下图片走 `image` 文件字段。普通用户发送 `quota_grant`/`subscription_grant` 之外的额度发放类型会被拒绝（仅管理员专用，`controller/support.go:186`）。图片会被压缩为 JPEG（`controller/support.go:89`）。

### 邀请与签到

- `GET /api/user/aff`（`controller/user.go:481`）：返回 `data` 为邀请码字符串。
- `POST /api/user/aff_transfer`（`controller/user.go:457`）：请求体 `{"quota":<int>}`（必填，`controller/user.go:453`）。未确认支付合规时失败（`controller/user.go:458`）。
- `GET /api/user/checkin`（`controller/checkin.go:16`）：功能未启用时返回失败「签到功能未启用」（`controller/checkin.go:18`）。`month` query 默认当前月（格式 `2006-01`）。返回 `data` 含 `enabled`、`min_quota`、`max_quota`、`min_user_quota`、`min_used_quota`、`daily_user_limit`、`daily_quota_limit`、`deductible_groups`、`eligibility`、`eligible`、`current_quota`、`current_used_quota`、`today_user_count`、`today_quota_awarded`、`daily_quota_remaining`、`stats`（`controller/checkin.go:40`）。
- `POST /api/user/checkin`（`controller/checkin.go:64`）：成功返回 `data.quota_awarded`、`data.checkin_date`。

### PUT /api/user/setting

`controller/user.go:1417`。请求体 `UpdateUserSettingRequest`（`controller/user.go:1401`）：`notify_type`、`quota_warning_threshold`、`webhook_url`、`webhook_secret`、`notification_email`、`bark_url`、`gotify_url`、`gotify_token`、`gotify_priority`、`upstream_model_update_notify_enabled`、`accept_unset_model_ratio_model`、`record_ip_log`、`exclude_from_leaderboard`。

校验：`notify_type` 必须是 `email`/`webhook`/`bark`/`gotify` 之一（`controller/user.go:1425`）；阈值必须 > 0（`controller/user.go:1431`）；webhook 类型要求合法 URL（`controller/user.go:1437`）；Bark/Gotify 要求 http(s) 前缀（`controller/user.go:1470`）。`upstream_model_update_notify_enabled` 仅在角色 ≥ 管理员时生效（`controller/user.go:1506`）。**`webhook_secret`、`gotify_token` 属凭据，写入用户设置后不随任何读取接口回显。**

### GET /api/user/models

`controller/user.go:649`。路径参数 `id` 非必需；解析失败时回退到当前登录用户（`controller/user.go:650`）。`group` query 可选，`auto` 表示虚拟分组。返回 `data` 为模型名数组（`service.GetGroupsEnabledModels`）。

### GET /api/user/self/groups 与 GET /api/user/groups

`controller/group.go:27` 与 `controller/group.go:15`。两者返回结构不同：`/api/user/groups`（未登录可调）返回按分组聚合的 map；`/api/user/self/groups` 同样返回 map，元素含 `ratio`、`desc`、`default_model`，并可能附加 `auto` 虚拟分组（`controller/group.go:49`）。

## 本文件未能确定的内容

- `POST /api/verify` 请求体 `service.VerificationInput` 的字段名（`service/` 包内定义，未核对）。
- `POST /api/user/2fa/setup` 返回的 `service.StartTwoFASetup` 结果字段名。
- `StripePayRequest`、`CreemPayRequest`、`WaffoPayRequest`、`RedemptionPurchaseRequest`、`Subscription*PayRequest` 的逐字段定义（仅在各自 controller 文件内声明，本文件只核对到其中被显式引用的字段）。
- `/api/oauth/state` 的 `intent` 取值仅确认到 `model.AuthFlowIntentLogin`/`AuthFlowIntentBind`/`AuthFlowIntentVerify` 三个常量名，其字面量值未在本次核对中读取。
- 各 `*RateLimit` 中间件的具体阈值与窗口（`middleware/rate-limit.go`）。
