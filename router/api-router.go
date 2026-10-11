package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"

	// Import oauth package to register providers via init()
	_ "github.com/QuantumNous/new-api/oauth"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.Use(middleware.RouteTag("api"))
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.BodyStorageCleanup()) // 清理请求体存储
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	anonymousRequestBodyLimit := middleware.AnonymousRequestBodyLimit()
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", anonymousRequestBodyLimit, controller.PostSetup)
		apiRouter.GET("/status", middleware.OptionalUserAuth(), controller.GetStatus)
		// Two independent status views converged on one path during the fork
		// merge. /status-check is the console's channel-status page: probe
		// results merged with passive traffic, per-model rows and the operator
		// announcement. The perf-status view (cache hit rate, TTFT history)
		// keeps its own path so both survive.
		apiRouter.GET("/status-check/perf", middleware.UserAuth(), controller.GetPerfMetricsStatus)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/landing_models", controller.GetLandingModels)
		apiRouter.GET("/background/random", controller.GetRandomBackground)
		apiRouter.GET("/pricing", middleware.HeaderNavModuleAuth("pricing"), controller.GetPricing)
		apiRouter.GET("/status-check", middleware.HeaderNavModulePublicOrUserAuth("status"), controller.GetStatusCheck)
		perfMetricsRoute := apiRouter.Group("/perf-metrics")
		perfMetricsRoute.Use(middleware.HeaderNavModulePublicOrUserAuth("pricing"))
		{
			perfMetricsRoute.GET("/summary", controller.GetPerfMetricsSummary)
			perfMetricsRoute.GET("", controller.GetPerfMetrics)
		}
		apiRouter.GET("/rankings", middleware.HeaderNavModuleAuth("rankings"), controller.GetRankings)
		apiRouter.GET("/rankings/users", middleware.HeaderNavModuleAuth("rankings"), controller.GetUserRankings)
		bangumiRoute := apiRouter.Group("/bangumi")
		bangumiRoute.Use(middleware.HeaderNavModulePublicOrUserAuth("bangumi"))
		{
			// The catalog reads an embedded file and reaches no upstream, so it is
			// registered ahead of the limiter: counting it would spend the budget
			// that exists to protect third-party quota, and the page needs it
			// before it can show anything at all.
			bangumiRoute.GET("/sources", controller.GetBangumiMediaSources)
		}
		bangumiRoute.Use(middleware.BangumiRateLimit())
		{
			bangumiRoute.GET("/sources/:sourceId/search", controller.SearchBangumiMediaSource)
			bangumiRoute.GET("/sources/:sourceId/subjects/:sourceSubjectId", controller.BrowseBangumiMediaSource)
			bangumiRoute.GET("/calendar", controller.GetBangumiCalendar)
			bangumiRoute.GET("/search", controller.GetBangumiSearch)
			bangumiRoute.GET("/ranking", controller.GetBangumiRanking)
			bangumiRoute.GET("/subjects/:id", controller.GetBangumiSubject)
			// Playback: the episode list comes from dandanplay's view of the same
			// bgm.tv subject id, and the releases come from Mikan. Both share the
			// bangumi module gate and rate limiter above, because both spend a
			// third party's quota on the visitor's behalf.
			bangumiRoute.GET("/danmaku/:episodeId", controller.GetBangumiDanmaku)
			bangumiRoute.GET("/releases/:subjectId", controller.GetBangumiReleases)
			// Mikan serves the .torrent with no CORS headers, so the browser
			// cannot fetch it itself; this same-origin hop is what lets
			// WebTorrent load the torrent at all.
			bangumiRoute.GET("/torrent/*path", controller.ProxyBangumiTorrent)
			// The segment is `:id`, not `:subjectId`: Gin's tree cannot carry two
			// different wildcard names at the same position, and `/subjects/:id`
			// is already registered above. A distinct name here panics at startup
			// rather than failing the request.
			bangumiRoute.GET("/subjects/:id/episodes", controller.GetBangumiEpisodes)
			// Resolving picks the actual playable target for one episode of one
			// source, which for a website source means parsing its player page and
			// for a torrent source means registering the release with the local
			// backend. Both spend upstream requests, so they sit behind the same
			// gate and limiter as the metadata routes above.
			bangumiRoute.GET("/sources/:sourceId/resolve", controller.ResolveBangumiMediaSource)
		}
		// The media route is registered outside the group above because it is
		// requested once per byte range rather than once per upstream call, and a
		// player issues a new range every time the viewer seeks.
		apiRouter.GET("/bangumi/media/:mediaId/:resourceId",
			middleware.HeaderNavModulePublicOrUserAuth("bangumi"),
			middleware.BangumiStreamRateLimit(),
			controller.GetBangumiSourceMedia)
		// Registered outside the group above, which carries the metadata limiter:
		// this route is counted per byte range rather than per upstream call,
		// because a player asks for a new range every time the viewer seeks.
		apiRouter.GET("/bangumi/stream/*path",
			middleware.HeaderNavModulePublicOrUserAuth("bangumi"),
			middleware.BangumiStreamRateLimit(),
			controller.GetBangumiStream)
		// The cover proxy reaches an upstream on the viewer's behalf, so it
		// carries the same module gate as every other bangumi route: with the
		// module off or set to require auth, it must not stay anonymously
		// reachable just because it is registered outside the group.
		apiRouter.GET("/bangumi-image/*path",
			middleware.HeaderNavModulePublicOrUserAuth("bangumi"),
			middleware.BangumiRateLimit(),
			controller.ProxyBangumiImage)
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.CaptchaCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.CaptchaCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.ResetPassword)
		// OAuth routes - specific routes must come before :provider wildcard
		apiRouter.POST("/oauth/state", middleware.OAuthRateLimit(), middleware.DisableCache(), middleware.TryUserAuth(), anonymousRequestBodyLimit, controller.GenerateOAuthCode)
		apiRouter.POST("/oauth/email/bind/start", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("account-security"), middleware.EmailVerificationRateLimit(), middleware.DisableCache(), controller.EmailBindStart)
		apiRouter.POST("/oauth/email/bind/resend", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("account-security"), middleware.EmailVerificationRateLimit(), middleware.DisableCache(), controller.EmailBindResend)
		apiRouter.POST("/oauth/email/bind", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("account-security"), middleware.DisableCache(), controller.EmailBind)
		// Non-standard OAuth (WeChat, Telegram) - keep original routes
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.WeChatAuth)
		apiRouter.POST("/oauth/wechat/bind", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.WeChatBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.TelegramLogin)
		apiRouter.POST("/oauth/telegram/bind/start", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.TelegramBindStart)
		apiRouter.GET("/oauth/telegram/bind/:flow_token", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.TelegramBind)
		// Standard OAuth providers (GitHub, Discord, OIDC, LinuxDO) - unified route
		apiRouter.GET("/oauth/:provider", middleware.OAuthRateLimit(), middleware.DisableCache(), middleware.TryUserAuth(), controller.HandleOAuth)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", anonymousRequestBodyLimit, controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", anonymousRequestBodyLimit, controller.CreemWebhook)
		apiRouter.POST("/waffo/webhook", anonymousRequestBodyLimit, controller.WaffoWebhook)
		apiRouter.POST("/nowpayments/webhook", anonymousRequestBodyLimit, controller.NowPaymentsWebhook)
		// :env separates test vs prod URLs so the operator can register each
		// in Pancake's matching webhook slot; handler enforces env match.
		apiRouter.POST("/waffo-pancake/webhook/:env", anonymousRequestBodyLimit, controller.WaffoPancakeWebhook)

		// Universal secure verification routes
		apiRouter.GET("/verify/methods", middleware.UserAuth(), middleware.DisableCache(), controller.GetVerificationMethods)
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.UniversalVerify)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/auth/refresh", middleware.SessionCookieOriginGuard(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.RefreshAuth)
			userRoute.POST("/auth/logout", middleware.SessionCookieOriginGuard(), middleware.DisableCache(), controller.AuthLogout)
			// CaptchaCheckRegister and RegisterRateLimit are the shared chain:
			// they dispatch to whichever captcha provider is configured, where
			// TurnstileCheck only knew one. The ticket route stays ours — it is
			// the anti-abuse handshake the register form performs first.
			// Signup carries three guards: RegisterRateLimit buckets registration apart
			// from other critical traffic, CaptchaCheckRegister dispatches to whichever
			// captcha provider is configured, and RegisterThrottle enforces the
			// IP-bound ticket plus the global rolling count that a rotated proxy pool
			// cannot escape.
			userRoute.POST("/register", middleware.RegisterRateLimit(), anonymousRequestBodyLimit, middleware.CaptchaCheckRegister(), middleware.RegisterThrottle(), controller.Register)
			userRoute.GET("/register_ticket", middleware.RegisterTicket(), controller.RegisterTicket)
			userRoute.GET("/login/encryption-key", middleware.DisableCache(), controller.GetPasswordEncryptionKey)
			userRoute.POST("/login", middleware.LoginRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, middleware.CaptchaCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.LoginRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.Verify2FALogin)
			userRoute.POST("/login/verify", middleware.LoginRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.VerifyLogin)
			userRoute.POST("/login/passkey/begin", middleware.LoginRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.LoginPasskeyBegin)
			userRoute.POST("/login/passkey/finish", middleware.LoginRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.LoginPasskeyFinish)
			userRoute.POST("/passkey/login/begin", middleware.LoginRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.LoginRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.POST("/epay/notify", anonymousRequestBodyLimit, controller.EpayNotify)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.DisableCache(), middleware.UserAuth())
			{
				selfRoute.GET("/sessions", middleware.DisableCache(), controller.GetLoginSessions)
				selfRoute.DELETE("/sessions/:sid", middleware.DisableCache(), controller.DeleteLoginSession)
				selfRoute.POST("/sessions/revoke-others", middleware.DisableCache(), controller.RevokeOtherLoginSessions)
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("access-token"), middleware.DisableCache(), controller.GenerateAccessToken)
				selfRoute.GET("/token/status", middleware.DisableCache(), controller.GetAccessTokenStatus)
				selfRoute.POST("/token", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("access-token"), middleware.DisableCache(), controller.GenerateAccessToken)
				selfRoute.DELETE("/token", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("access-token"), middleware.DisableCache(), controller.RevokeAccessToken)
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", middleware.DisableCache(), controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", middleware.DisableCache(), controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", middleware.DisableCache(), controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", middleware.DisableCache(), controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", middleware.DisableCache(), controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.GET("/redemption/self", controller.GetUserRedemptions)
				selfRoute.POST("/redemption/:id/refund", middleware.UserCriticalRateLimit("redemption-refund"), controller.RefundUserRedemption)
				// Redemption is keyed to the authenticated user so one busy shared IP
				// cannot exhaust the bucket for unrelated users.
				selfRoute.POST("/topup", middleware.UserCriticalRateLimit("redemption"), middleware.CaptchaCheckRedemption(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				// Buying redemption codes spends real money, so the routes that
				// move it carry the same critical rate limit as the other
				// payment routes. The options read is free.
				selfRoute.GET("/redemption/purchase", controller.GetRedemptionPurchaseOptions)
				selfRoute.POST("/redemption/purchase/amount", middleware.CriticalRateLimit(), controller.RequestRedemptionPurchaseAmount)
				selfRoute.POST("/redemption/purchase", middleware.CriticalRateLimit(), controller.RequestRedemptionPurchase)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/waffo/amount", controller.RequestWaffoAmount)
				selfRoute.POST("/waffo/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPay)
				selfRoute.POST("/waffo-pancake/amount", controller.RequestWaffoPancakeAmount)
				selfRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPancakePay)
				selfRoute.POST("/monero/pay", middleware.CriticalRateLimit(), controller.RequestMoneroPay)
				selfRoute.GET("/monero/payment", controller.GetMoneroPaymentStatus)
				selfRoute.POST("/nowpayments/pay", middleware.CriticalRateLimit(), controller.RequestNowPaymentsPay)
				selfRoute.GET("/nowpayments/payment", controller.GetNowPaymentsPaymentStatus)
				selfRoute.POST("/aff_transfer", middleware.UserCriticalRateLimit("aff-transfer"), controller.TransferAffQuota)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", middleware.DisableCache(), controller.Setup2FA)
				selfRoute.POST("/2fa/enable", middleware.DisableCache(), controller.Enable2FA)
				selfRoute.POST("/2fa/disable", middleware.DisableCache(), controller.Disable2FA)
				selfRoute.PUT("/2fa/login-verification", middleware.DisableCache(), controller.UpdateLoginTwoFA)
				selfRoute.POST("/2fa/backup_codes", middleware.DisableCache(), controller.RegenerateBackupCodes)

				// Check-in routes
				selfRoute.GET("/checkin", controller.GetCheckinStatus)
				selfRoute.POST("/checkin", middleware.CaptchaCheckCheckin(), controller.DoCheckin)

				// Custom wallpaper (paid). Setting one moves money, so it
				// carries the same critical rate limit as the other
				// balance-spending routes; reading and clearing are free.
				selfRoute.GET("/wallpaper", controller.GetUserWallpaper)
				selfRoute.PUT("/wallpaper", middleware.CriticalRateLimit(), controller.UpdateUserWallpaper)
				selfRoute.DELETE("/wallpaper", controller.DeleteUserWallpaper)
				// Resolving a Wallpaper Engine link makes an outbound request, so
				// it shares the paid path's rate limit.
				selfRoute.POST("/wallpaper/resolve", middleware.CriticalRateLimit(), controller.ResolveWallpaperSource)

				// Custom OAuth bindings
				selfRoute.GET("/oauth/bindings", controller.GetUserOAuthBindings)
				selfRoute.DELETE("/oauth/bindings/:provider_id", controller.UnbindCustomOAuth)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/topup", controller.GetAllTopUps)
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id/oauth/bindings", controller.GetUserOAuthBindingsByAdmin)
				adminRoute.DELETE("/:id/oauth/bindings/:provider_id", controller.UnbindCustomOAuthByAdmin)
				adminRoute.DELETE("/:id/bindings/:binding_type", controller.AdminClearUserBinding)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.POST("/manage/batch", controller.BatchManageUsers)
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)

				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)

				// Custom wallpaper read/clear for any user. Setting a paid
				// wallpaper always spends the owner's own balance, so it stays
				// owner-only.
				adminRoute.GET("/:id/wallpaper", controller.AdminGetUserWallpaper)
				adminRoute.DELETE("/:id/wallpaper", controller.AdminDeleteUserWallpaper)
			}
		}

		// Subscription billing (plans, purchase, admin management)
		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth())
		{
			subscriptionRoute.GET("/plans", controller.GetSubscriptionPlans)
			subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)
			subscriptionRoute.PUT("/self/preference", controller.UpdateSubscriptionPreference)
			subscriptionRoute.POST("/balance/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestBalancePay)
			subscriptionRoute.POST("/epay/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestEpay)
			subscriptionRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestStripePay)
			subscriptionRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestCreemPay)
			subscriptionRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestWaffoPancakePay)
			subscriptionRoute.POST("/nowpayments/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestNowPaymentsPay)
		}
		subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
		subscriptionAdminRoute.Use(middleware.AdminAuth())
		{
			subscriptionAdminRoute.GET("/plans", controller.AdminListSubscriptionPlans)
			subscriptionAdminRoute.POST("/plans", controller.AdminCreateSubscriptionPlan)
			subscriptionAdminRoute.PUT("/plans/:id", controller.AdminUpdateSubscriptionPlan)
			subscriptionAdminRoute.PATCH("/plans/:id", controller.AdminUpdateSubscriptionPlanStatus)
			subscriptionAdminRoute.DELETE("/plans/:id", controller.AdminDeleteSubscriptionPlan)
			subscriptionAdminRoute.POST("/bind", controller.AdminBindSubscription)
			subscriptionAdminRoute.POST("/plans/:id/subscriptions/reset", controller.AdminResetPlanSubscriptions)

			// User subscription management (admin)
			subscriptionAdminRoute.GET("/users/:id/subscriptions", controller.AdminListUserSubscriptions)
			subscriptionAdminRoute.POST("/users/:id/subscriptions", controller.AdminCreateUserSubscription)
			subscriptionAdminRoute.POST("/users/:id/subscriptions/reset", controller.AdminResetUserSubscriptionsByPlan)
			subscriptionAdminRoute.POST("/user_subscriptions/:id/invalidate", controller.AdminInvalidateUserSubscription)
			subscriptionAdminRoute.DELETE("/user_subscriptions/:id", controller.AdminDeleteUserSubscription)
		}

		// In-console support inbox. Users have one persistent "服务支持" thread;
		// administrators can browse threads and issue audited grants.
		supportRoute := apiRouter.Group("/support")
		supportRoute.Use(middleware.UserAuth(), middleware.SupportEnabled())
		{
			supportRoute.GET("/unread", middleware.DisableCache(), controller.GetSupportUnread)
			supportRoute.GET("/conversation", middleware.DisableCache(), controller.GetSupportConversation)
			supportRoute.GET("/orders", middleware.DisableCache(), controller.GetSupportOrders)
			supportRoute.POST("/messages", middleware.UploadRateLimit(), middleware.DisableCache(), controller.SendSupportMessage)
		}
		supportAdminRoute := apiRouter.Group("/support/admin")
		supportAdminRoute.Use(middleware.AdminAuth(), middleware.SupportEnabled())
		{
			supportAdminRoute.GET("/conversations", middleware.DisableCache(), controller.AdminListSupportConversations)
			supportAdminRoute.GET("/conversations/:id", middleware.DisableCache(), controller.AdminGetSupportConversation)
			supportAdminRoute.POST("/conversations/:id/messages", middleware.UploadRateLimit(), middleware.DisableCache(), controller.AdminSendSupportMessage)
			supportAdminRoute.POST("/conversations/:id/grant-quota", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.AdminGrantSupportQuota)
			supportAdminRoute.POST("/conversations/:id/grant-subscription", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.AdminGrantSupportSubscription)
			supportAdminRoute.POST("/messages/:id/complete-order", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.AdminCompleteSupportOrder)
		}

		// Refund requests (plan refunds and general cash refunds). Every route
		// that creates a refund or moves it forward is admin-gated; the user
		// route is read-only and scoped to the caller.
		refundRoute := apiRouter.Group("/refund")
		refundRoute.Use(middleware.UserAuth())
		{
			refundRoute.GET("/self", controller.GetUserRefundRequests)
		}
		refundAdminRoute := apiRouter.Group("/refund/admin")
		refundAdminRoute.Use(middleware.AdminAuth())
		{
			refundAdminRoute.GET("/", controller.AdminListRefundRequests)
			refundAdminRoute.POST("/plan", controller.AdminCreatePlanRefundRequest)
			refundAdminRoute.POST("/cash", controller.AdminCreateCashRefundRequest)
			refundAdminRoute.POST("/:id/advance", controller.AdminAdvanceRefundRequest)
		}

		// Subscription payment callbacks (no auth)
		apiRouter.POST("/subscription/epay/notify", anonymousRequestBodyLimit, controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/return", controller.SubscriptionEpayReturn)
		apiRouter.POST("/subscription/epay/return", anonymousRequestBodyLimit, controller.SubscriptionEpayReturn)
		// Upstream price sentinel. The report and the manual run are root-only;
		// the edit page is reached with the token mailed with the report, so a
		// buyer of the link still needs the admin access token to write.
		priceSentinelRoute := apiRouter.Group("/price-sentinel")
		priceSentinelRoute.Use(middleware.RootAuth())
		{
			priceSentinelRoute.GET("/report", controller.GetPriceSentinelReport)
			priceSentinelRoute.POST("/run", controller.RunPriceSentinelNow)
		}
		// Neither route carries the shared critical limiter. Its budget is 20
		// requests per 20 minutes across every critical operation, and applying a
		// digest legitimately means writing many prices in one sitting - the page
		// exists to be used repeatedly, so that budget only locks the operator out
		// of their own report. The authorization that matters is unchanged: every
		// write needs a root session *and* a mailed link token, and each one is
		// audited.
		priceSentinelEditRoute := apiRouter.Group("/price-sentinel/edit")
		{
			// Checking the link needs only the mailed token, so the page can show
			// the report before the operator authenticates.
			priceSentinelEditRoute.POST("/validate", controller.ValidatePriceSentinelToken)
			// Writing a price needs the mailed token *and* a root session, so a
			// leaked or forwarded link is not on its own enough to change billing.
			priceSentinelEditRoute.POST("/apply", middleware.RootAuth(), controller.ApplyPriceSentinelEdit)
			// Applying a whole report at once, marked up by a margin, so the
			// operator does not have to write dozens of rows by hand.
			priceSentinelEditRoute.POST("/sync", middleware.RootAuth(), controller.SyncPriceSentinelPrices)
		}
		// Registrar. The account archive holds upstream login secrets
		// (fingerprints) and live API keys, so every route is root-only.
		registrarRoute := apiRouter.Group("/registrar")
		registrarRoute.Use(middleware.RootAuth())
		{
			registrarRoute.GET("/providers", controller.GetRegistrarProviders)
			registrarRoute.PUT("/providers", controller.UpdateRegistrarProviders)
			registrarRoute.POST("/providers/:id/run", controller.RunRegistrarProvider)
			registrarRoute.GET("/accounts", controller.ListRegistrarAccounts)
			registrarRoute.GET("/accounts/export", controller.ExportRegistrarAccounts)
			registrarRoute.POST("/accounts/import", controller.ImportRegistrarAccounts)
		}
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.GET("/request_policy", controller.GetRequestPolicy)
			optionRoute.PATCH("/request_policy", controller.UpdateRequestPolicy)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.PUT("/passkey/domains", controller.UpdatePasskeyDomains)
			optionRoute.GET("/model_pricing", controller.GetModelPricingConfig)
			optionRoute.PATCH("/model_pricing", controller.UpdateModelPricingConfig)
			optionRoute.POST("/smtp/test", controller.SendSMTPTestEmail)
			optionRoute.DELETE("/support", controller.ClearSupportData)
			optionRoute.POST("/model_pricing/preview", controller.PreviewModelPricing)
			optionRoute.POST("/payment_compliance", controller.ConfirmPaymentCompliance)
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
			optionRoute.DELETE("/channel_affinity_cache", controller.ClearChannelAffinityCache)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.GET("/waffo-pancake/catalog", controller.ListWaffoPancakeCatalog)
			optionRoute.POST("/waffo-pancake/pair", controller.CreateWaffoPancakePair)
			optionRoute.POST("/waffo-pancake/save", controller.SaveWaffoPancake)
			optionRoute.POST("/waffo-pancake/subscription-product", controller.CreateWaffoPancakeSubscriptionProduct)
			optionRoute.GET("/waffo-pancake/subscription-product-options", controller.ListWaffoPancakeSubscriptionProductOptions)
		}

		// Outbound proxy pool (admin). The pool is an external service, so the
		// read routes expose only reachability/count while assign/report write
		// channel state. Admin-only: an unprivileged caller must not be able to
		// route every channel's traffic through a URL they control.
		proxyPoolRoute := apiRouter.Group("/proxy_pool")
		proxyPoolRoute.Use(middleware.AdminAuth())
		{
			proxyPoolRoute.GET("/status", middleware.RequirePermission(authz.ChannelRead), controller.GetProxyPoolStatus)
			proxyPoolRoute.GET("/all", middleware.RequirePermission(authz.ChannelRead), controller.ListProxyPoolProxies)
			proxyPoolRoute.POST("/assign", middleware.RequirePermission(authz.ChannelWrite), controller.AssignProxyPoolProxies)
			proxyPoolRoute.POST("/report_bad", middleware.RequirePermission(authz.ChannelOperate), controller.ReportBadProxyPoolProxy)
		}

		// Custom OAuth provider management (root only)
		customOAuthRoute := apiRouter.Group("/custom-oauth-provider")
		customOAuthRoute.Use(middleware.RootAuth())
		{
			customOAuthRoute.POST("/discovery", controller.FetchCustomOAuthDiscovery)
			customOAuthRoute.GET("/", controller.GetCustomOAuthProviders)
			customOAuthRoute.GET("/:id", controller.GetCustomOAuthProvider)
			customOAuthRoute.POST("/", controller.CreateCustomOAuthProvider)
			customOAuthRoute.PUT("/:id", controller.UpdateCustomOAuthProvider)
			customOAuthRoute.DELETE("/:id", controller.DeleteCustomOAuthProvider)
		}

		// Linked service management (root only)
		linkedServiceRoute := apiRouter.Group("/linked-service")
		linkedServiceRoute.Use(middleware.RootAuth())
		{
			linkedServiceRoute.GET("/", controller.ListLinkedServices)
			linkedServiceRoute.GET("/:id", controller.GetLinkedService)
			linkedServiceRoute.POST("/", controller.CreateLinkedService)
			linkedServiceRoute.PUT("/:id", controller.UpdateLinkedService)
			linkedServiceRoute.DELETE("/:id", controller.DeleteLinkedService)
			linkedServiceRoute.POST("/:id/sync-password", controller.SyncLinkedServicePassword)
		}

		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", controller.GetPerformanceStats)
			performanceRoute.DELETE("/disk_cache", controller.ClearDiskCache)
			performanceRoute.POST("/reset_stats", controller.ResetPerformanceStats)
			performanceRoute.POST("/gc", controller.ForceGC)
			performanceRoute.GET("/logs", controller.GetLogFiles)
			performanceRoute.DELETE("/logs", controller.CleanupLogFiles)
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		taskPluginRoute := apiRouter.Group("/plugin/task")
		taskPluginRoute.Use(middleware.RootAuth())
		{
			taskPluginRoute.GET("", controller.ListTaskPlugins)
			taskPluginRoute.POST("", controller.UploadTaskPlugin)
			taskPluginRoute.PUT("", controller.UploadTaskPlugin)
			taskPluginRoute.GET("/runtime/status", controller.GetTaskPluginRuntime)
			taskPluginRoute.GET("/marketplace/sources", controller.GetTaskPluginMarketplaceSources)
			taskPluginRoute.PUT("/marketplace/sources", controller.UpdateTaskPluginMarketplaceSources)
			taskPluginRoute.GET("/:key", controller.GetTaskPlugin)
			taskPluginRoute.GET("/:key/icon", controller.GetTaskPluginIcon)
			taskPluginRoute.GET("/:key/versions", controller.GetTaskPluginVersions)
			taskPluginRoute.POST("/:key/activate", controller.ActivateTaskPlugin)
			taskPluginRoute.POST("/:key/status", controller.SetTaskPluginStatus)
			taskPluginRoute.POST("/:key/dryrun", controller.DryRunTaskPlugin)
			taskPluginRoute.DELETE("/:key/versions/:version", controller.DeleteTaskPluginVersion)
		}
		apiRouter.GET("/task_plugin_options", middleware.AdminAuth(), middleware.RequirePermission(authz.TaskPluginBind), controller.GetTaskPluginOptions)
		registerChannelRoutes(apiRouter)
		registerAuthzRoutes(apiRouter)
		tokenAdminRoute := apiRouter.Group("/token")
		tokenAdminRoute.Use(middleware.AdminAuth())
		{
			tokenAdminRoute.GET("/group-names", controller.GetTokenGroupNames)
			tokenAdminRoute.POST("/group/migrate", controller.MigrateTokenGroup)
		}
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/rpm", controller.GetTokenRPM)
			tokenRoute.GET("/ip-stats", controller.GetTokenIpStats)
			tokenRoute.GET("/search", middleware.SearchRateLimit(), controller.SearchTokens)
			tokenRoute.GET("/auto-groups", controller.GetTokenAutoGroups)
			tokenRoute.GET("/:id/auto-routes", controller.GetTokenAutoRoutes)
			tokenRoute.GET("/:id/auto-routes/status", controller.GetTokenAutoRouteStatus)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/:id/key", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKey)
			tokenRoute.POST("/:id/reset-used-quota", controller.ResetTokenUsedQuota)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
			tokenRoute.POST("/batch/keys", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKeysBatch)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuthReadOnly())
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			// AdminAuth alone let any administrator mint codes, including one
			// deliberately scoped to read-only elsewhere. Creating a code grants
			// spendable balance, so it carries its own permission and can be
			// withheld from an individual administrator.
			redemptionRoute.GET("/", middleware.RequirePermission(authz.RedemptionRead), controller.GetAllRedemptions)
			redemptionRoute.GET("/search", middleware.RequirePermission(authz.RedemptionRead), controller.SearchRedemptions)
			redemptionRoute.GET("/:id", middleware.RequirePermission(authz.RedemptionRead), controller.GetRedemption)
			redemptionRoute.POST("/", middleware.RequirePermission(authz.RedemptionCreateCode), controller.AddRedemption)
			redemptionRoute.PUT("/", middleware.RequirePermission(authz.RedemptionCreateCode), controller.UpdateRedemption)
			redemptionRoute.POST("/batch", middleware.RequirePermission(authz.RedemptionDeleteCode), controller.DeleteRedemptionBatch)
			redemptionRoute.DELETE("/invalid", middleware.RequirePermission(authz.RedemptionDeleteCode), controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", middleware.RequirePermission(authz.RedemptionDeleteCode), controller.DeleteRedemption)
		}
		apiRouter.GET("/audit", middleware.DisableCache(), middleware.AdminAuth(), middleware.RequirePermission(authz.AuditRead), controller.GetAuditLogs)
		apiRouter.GET("/audit/export", middleware.DisableCache(), middleware.AdminAuth(), middleware.RequirePermission(authz.AuditRead), controller.ExportAuditLogsCSV)
		apiRouter.GET("/audit/self", middleware.DisableCache(), middleware.UserAuth(), controller.GetAuditLogs)
		apiRouter.GET("/audit/self/export", middleware.DisableCache(), middleware.UserAuth(), controller.ExportAuditLogsCSV)
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		logRoute.GET("/export", middleware.AdminAuth(), controller.ExportLogsCSV)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/self/export", middleware.UserAuth(), controller.ExportLogsCSV)
		logRoute.GET("/self/search", middleware.UserAuth(), middleware.SearchRateLimit(), controller.SearchUserLogs)

		// 内容审查：请求/回答正文的查看与下载。AdminAuth（root 及管理员）可见；
		// 普通用户的 self 日志接口不返回正文，这张表也不在任何用户查询路径上。
		contentReviewRoute := apiRouter.Group("/content-review")
		contentReviewRoute.Use(middleware.AdminAuth())
		{
			contentReviewRoute.GET("/status", controller.GetContentReviewStatus)
			contentReviewRoute.GET("/", controller.GetContentReviewList)
			contentReviewRoute.GET("/export", controller.DownloadContentReviewList)
			contentReviewRoute.GET("/:id", controller.GetContentReviewDetail)
			contentReviewRoute.GET("/:id/download", controller.DownloadContentReviewDetail)
			contentReviewRoute.DELETE("/:id", controller.DeleteContentReview)
			contentReviewRoute.POST("/cleanup", controller.CleanupContentReview)
		}

		systemTaskRoute := apiRouter.Group("/system-task")
		systemTaskRoute.Use(middleware.RootAuth())
		{
			systemTaskRoute.POST("/log-cleanup", controller.CreateLogCleanupSystemTask)
			systemTaskRoute.GET("/list", controller.ListSystemTasks)
			systemTaskRoute.DELETE("/history", controller.DeleteSystemTaskHistory)
			systemTaskRoute.GET("/current", controller.GetCurrentSystemTask)
			systemTaskRoute.GET("/:task_id", controller.GetSystemTask)
		}
		systemInfoRoute := apiRouter.Group("/system-info")
		systemInfoRoute.Use(middleware.RootAuth())
		{
			systemInfoRoute.GET("/instances", controller.ListSystemInstances)
			systemInfoRoute.DELETE("/stale-instances", controller.DeleteStaleSystemInstances)
			systemInfoRoute.DELETE("/instances/:node_name", controller.DeleteStaleSystemInstance)
		}

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/users", middleware.AdminAuth(), controller.GetQuotaDatesByUser)
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)
		dataRoute.GET("/flow", middleware.AdminAuth(), controller.GetAllFlowQuotaDates)
		dataRoute.GET("/flow/self", middleware.UserAuth(), controller.GetUserFlowQuotaDates)

		logRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			logRoute.GET("/token", middleware.TokenAuthReadOnly(), controller.GetLogByKey)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup)
		}

		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)
		mjRoute.GET("/self/export", middleware.DisableCache(), middleware.UserAuth(), controller.ExportMidjourneyCSV)
		mjRoute.GET("/export", middleware.DisableCache(), middleware.AdminAuth(), controller.ExportMidjourneyCSV)
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("/self/export", middleware.DisableCache(), middleware.UserAuth(), controller.ExportTaskCSV)
			taskRoute.GET("/export", middleware.DisableCache(), middleware.AdminAuth(), controller.ExportTaskCSV)
			taskRoute.GET("", middleware.AdminAuth(), controller.GetAllTask)
			taskRoute.GET("/:task_id/artifacts", middleware.UserAuth(), controller.GetDashboardTaskArtifacts)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.POST("/operations/preview", controller.PreviewVendorOperation)
			vendorRoute.POST("/operations", controller.ApplyVendorOperation)
			vendorRoute.GET("/", controller.GetAllVendors)
			vendorRoute.GET("/search", controller.SearchVendors)
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			vendorRoute.POST("/", controller.CreateVendorMeta)
			vendorRoute.PUT("/", controller.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta)
		}

		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)
			modelsRoute.POST("/delete", controller.BatchDeleteModelMeta)
			modelsRoute.GET("/missing", controller.GetMissingModels)
			modelsRoute.DELETE("/delete-unused", controller.DeleteUnusedModelsMeta)
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			modelsRoute.GET("/:id", controller.GetModelMeta)
			modelsRoute.POST("/", controller.CreateModelMeta)
			modelsRoute.PUT("/", controller.UpdateModelMeta)
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)
		}

		// Deployments (model deployment management)
		deploymentsRoute := apiRouter.Group("/deployments")
		deploymentsRoute.Use(middleware.AdminAuth())
		{
			deploymentsRoute.GET("/settings", controller.GetModelDeploymentSettings)
			deploymentsRoute.POST("/settings/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/", controller.GetAllDeployments)
			deploymentsRoute.GET("/search", controller.SearchDeployments)
			deploymentsRoute.POST("/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/hardware-types", controller.GetHardwareTypes)
			deploymentsRoute.GET("/locations", controller.GetLocations)
			deploymentsRoute.GET("/available-replicas", controller.GetAvailableReplicas)
			deploymentsRoute.POST("/price-estimation", controller.GetPriceEstimation)
			deploymentsRoute.GET("/check-name", controller.CheckClusterNameAvailability)
			deploymentsRoute.POST("/", controller.CreateDeployment)

			deploymentsRoute.GET("/:id", controller.GetDeployment)
			deploymentsRoute.GET("/:id/logs", controller.GetDeploymentLogs)
			deploymentsRoute.GET("/:id/containers", controller.ListDeploymentContainers)
			deploymentsRoute.GET("/:id/containers/:container_id", controller.GetContainerDetails)
			deploymentsRoute.PUT("/:id", controller.UpdateDeployment)
			deploymentsRoute.PUT("/:id/name", controller.UpdateDeploymentName)
			deploymentsRoute.POST("/:id/extend", controller.ExtendDeployment)
			deploymentsRoute.DELETE("/:id", controller.DeleteDeployment)
		}
	}
}
