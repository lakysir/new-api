package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

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
		apiRouter.GET("/status", controller.GetStatus)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/pricing", middleware.HeaderNavModuleAuth("pricing"), controller.GetPricing)
		apiRouter.GET("/pricing/model", middleware.HeaderNavModuleAuth("pricing"), controller.GetPricingModel)
		perfMetricsRoute := apiRouter.Group("/perf-metrics")
		perfMetricsRoute.Use(middleware.HeaderNavModulePublicOrUserAuth("pricing"))
		{
			perfMetricsRoute.GET("/summary", controller.GetPerfMetricsSummary)
			perfMetricsRoute.GET("", controller.GetPerfMetrics)
		}
		apiRouter.GET("/rankings", middleware.HeaderNavModuleAuth("rankings"), controller.GetRankings)
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.ResetPassword)
		// OAuth routes - specific routes must come before :provider wildcard
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), controller.GenerateOAuthCode)
		apiRouter.POST("/oauth/email/bind", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.EmailBind)
		// Non-standard OAuth (WeChat, Telegram) - keep original routes
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.POST("/oauth/wechat/bind", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.WeChatBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), controller.TelegramLogin)
		apiRouter.GET("/oauth/telegram/bind", middleware.CriticalRateLimit(), controller.TelegramBind)
		// Standard OAuth providers (GitHub, Discord, OIDC, LinuxDO) - unified route
		apiRouter.GET("/oauth/:provider", middleware.CriticalRateLimit(), controller.HandleOAuth)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", anonymousRequestBodyLimit, controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", anonymousRequestBodyLimit, controller.CreemWebhook)
		apiRouter.POST("/waffo/webhook", anonymousRequestBodyLimit, controller.WaffoWebhook)
		// :env separates test vs prod URLs so the operator can register each
		// in Pancake's matching webhook slot; handler enforces env match.
		apiRouter.POST("/waffo-pancake/webhook/:env", anonymousRequestBodyLimit, controller.WaffoPancakeWebhook)

		// Universal secure verification routes
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.UniversalVerify)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.Verify2FALogin)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.GET("/logout", controller.Logout)
			userRoute.POST("/epay/notify", anonymousRequestBodyLimit, controller.EpayNotify)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", middleware.CriticalRateLimit(), controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", controller.GenerateAccessToken)
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/waffo/amount", controller.RequestWaffoAmount)
				selfRoute.POST("/waffo/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPay)
				selfRoute.POST("/waffo-pancake/amount", controller.RequestWaffoPancakeAmount)
				selfRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPancakePay)
				selfRoute.POST("/aff_transfer", controller.TransferAffQuota)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)
				selfRoute.GET("/invoice", controller.GetInvoiceOverview)
				selfRoute.POST("/invoice", middleware.CriticalRateLimit(), controller.CreateInvoiceApplication)
				selfRoute.POST("/invoice/:id/cancel", controller.CancelInvoiceApplication)
				selfRoute.GET("/invoice/profiles", controller.ListInvoiceProfiles)
				selfRoute.POST("/invoice/profiles", controller.SaveInvoiceProfile)
				selfRoute.DELETE("/invoice/profiles/:id", controller.DeleteInvoiceProfile)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", controller.Setup2FA)
				selfRoute.POST("/2fa/enable", controller.Enable2FA)
				selfRoute.POST("/2fa/disable", controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", controller.RegenerateBackupCodes)

				// Check-in routes
				selfRoute.GET("/checkin", controller.GetCheckinStatus)
				selfRoute.POST("/checkin", middleware.TurnstileCheck(), controller.DoCheckin)

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
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)
				adminRoute.GET("/invoice/applications", controller.AdminListInvoiceApplications)
				adminRoute.POST("/invoice/applications/:id/review", controller.AdminReviewInvoiceApplication)
				adminRoute.POST("/invoice/applications/:id/sent", controller.AdminMarkInvoiceSent)

				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)
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
		}
		subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
		subscriptionAdminRoute.Use(middleware.AdminAuth())
		{
			subscriptionAdminRoute.GET("/plans", controller.AdminListSubscriptionPlans)
			subscriptionAdminRoute.POST("/plans", controller.AdminCreateSubscriptionPlan)
			subscriptionAdminRoute.PUT("/plans/:id", controller.AdminUpdateSubscriptionPlan)
			subscriptionAdminRoute.PATCH("/plans/:id", controller.AdminUpdateSubscriptionPlanStatus)
			subscriptionAdminRoute.POST("/bind", controller.AdminBindSubscription)

			// User subscription management (admin)
			subscriptionAdminRoute.GET("/users/:id/subscriptions", controller.AdminListUserSubscriptions)
			subscriptionAdminRoute.POST("/users/:id/subscriptions", controller.AdminCreateUserSubscription)
			subscriptionAdminRoute.POST("/user_subscriptions/:id/invalidate", controller.AdminInvalidateUserSubscription)
			subscriptionAdminRoute.DELETE("/user_subscriptions/:id", controller.AdminDeleteUserSubscription)
		}

		// Subscription payment callbacks (no auth)
		apiRouter.POST("/subscription/epay/notify", anonymousRequestBodyLimit, controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/return", controller.SubscriptionEpayReturn)
		apiRouter.POST("/subscription/epay/return", anonymousRequestBodyLimit, controller.SubscriptionEpayReturn)
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.POST("/payment_compliance", controller.ConfirmPaymentCompliance)
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
			optionRoute.DELETE("/channel_affinity_cache", controller.ClearChannelAffinityCache)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.POST("/migrate_console_setting", controller.MigrateConsoleSetting) // 用于迁移检测的旧键，下个版本会删除
			optionRoute.GET("/waffo-pancake/catalog", controller.ListWaffoPancakeCatalog)
			optionRoute.POST("/waffo-pancake/pair", controller.CreateWaffoPancakePair)
			optionRoute.POST("/waffo-pancake/save", controller.SaveWaffoPancake)
			optionRoute.POST("/waffo-pancake/subscription-product", controller.CreateWaffoPancakeSubscriptionProduct)
			optionRoute.GET("/waffo-pancake/subscription-product-options", controller.ListWaffoPancakeSubscriptionProductOptions)
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
		registerChannelRoutes(apiRouter)
		registerAuthzRoutes(apiRouter)
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", middleware.SearchRateLimit(), controller.SearchTokens)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/:id/key", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKey)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
			tokenRoute.POST("/batch/keys", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKeysBatch)
		}

		scriptRoute := apiRouter.Group("/scripts")
		{
			scriptRoute.GET("/square", controller.ListScriptSquare)
			scriptRoute.GET("/square/:id", controller.GetScriptSquareDetail)
			// Platform script-signing public key (public; used by plugins to
			// verify market-script signatures — no secret exposed).
			scriptRoute.GET("/platform-key", controller.GetPlatformScriptKey)
			// Provider offers for a script version (price + online + quota).
			// TryUserAuth (optional): populates the caller id when logged in so a
			// provider sees their own disabled nodes, without rejecting anonymous
			// browsing of public offers.
			scriptRoute.GET("/:id/offers", middleware.TryUserAuth(), controller.ListScriptOffers)
			// Target-site categories (public list; operator create/config).
			scriptRoute.GET("/categories", controller.ListCategories)
			// Public: caller-facing API doc for a bridged model (schemas +
			// defaults, no secrets). Backs the model-square doc entry.
			scriptRoute.GET("/model-doc/:model_name", controller.GetScriptModelDoc)

			scriptUserRoute := scriptRoute.Group("/")
			scriptUserRoute.Use(middleware.UserAuth())
			{
				scriptUserRoute.GET("/mine", controller.ListMyScripts)
				scriptUserRoute.POST("/mine", controller.SaveMyScriptDraft)
				scriptUserRoute.GET("/mine/:id", controller.GetMyScript)
				scriptUserRoute.PUT("/mine/:id", controller.SaveMyScriptDraft)
				scriptUserRoute.DELETE("/mine/:id", controller.DeleteMyScript)

				// Trusted version lifecycle: submit for review, publish an
				// immutable signed version, and read version history.
				scriptUserRoute.POST("/mine/:id/submit-review", controller.SubmitScriptForReview)
				scriptUserRoute.POST("/mine/:id/publish-version", controller.PublishScriptVersion)
				scriptUserRoute.GET("/mine/:id/versions", controller.ListScriptVersions)
				scriptUserRoute.GET("/:id/versions/available", controller.ListExecutableScriptVersions)
			}

			// Operator review + revoke, and fixed-version fetch used by order
			// execution (signature + hash verified plugin-side).
			scriptAdminRoute := scriptRoute.Group("/")
			scriptAdminRoute.Use(middleware.AdminAuth())
			{
				scriptAdminRoute.POST("/categories", controller.CreateCategory)
				scriptAdminRoute.POST("/categories/:id/balance-script", controller.SetCategoryBalanceScript)
				scriptAdminRoute.GET("/pending", controller.ListPendingScripts)
				scriptAdminRoute.GET("/versions/published", controller.ListPublishedScriptVersions)
				scriptAdminRoute.POST("/:id/review", controller.ReviewScriptDecision)
				scriptAdminRoute.POST("/:id/versions/:version/revoke", controller.RevokeScriptVersion)
				scriptAdminRoute.DELETE("/:id/versions/:version", controller.DeleteScriptVersion)
				scriptAdminRoute.PUT("/:id/versions/:version/pricing", controller.UpdateScriptVersionPricing)
				// Publish a script version as a callable new-api model (bridge to
				// /v1/videos etc.), list bindings, and unpublish by model name.
				scriptAdminRoute.POST("/:id/versions/:version/publish-model", controller.PublishScriptAsModel)
				scriptAdminRoute.GET("/model-bindings", controller.ListScriptModelBindings)
				scriptAdminRoute.DELETE("/model-bindings/:model_name", controller.UnpublishScriptModel)
				// Generate/rotate the platform Ed25519 script-signing key.
				scriptAdminRoute.POST("/signing-key/generate", controller.GenerateScriptSigningKey)
				// Platform revenue summary (day/week/month/lifetime service fees).
				scriptAdminRoute.GET("/platform-earnings", controller.GetPlatformEarnings)
				// Withdraw platform revenue to the admin's main wallet quota.
				scriptAdminRoute.POST("/platform-earnings/withdraw", middleware.CriticalRateLimit(), controller.WithdrawPlatformEarnings)
			}

			scriptVersionRoute := scriptRoute.Group("/")
			// Plugin fetches fixed versions with its API key; console uses session.
			scriptVersionRoute.Use(middleware.CORS(), middleware.DeviceOrUserAuth())
			{
				scriptVersionRoute.GET("/:id/versions/:version", controller.GetFixedScriptVersion)
			}
		}

		// Browser-extension plugin releases: public version-check + download
		// (CORS so the extension can call from its own origin), admin upload.
		pluginRoute := apiRouter.Group("/plugin")
		pluginRoute.Use(middleware.CORS())
		{
			pluginRoute.GET("/latest", controller.GetLatestPluginRelease)
			pluginRoute.GET("/download", controller.DownloadLatestPluginRelease)
			pluginRoute.POST("/upload", middleware.AdminAuth(), middleware.CriticalRateLimit(), controller.UploadPluginRelease)
		}

		assetRoute := apiRouter.Group("/user-assets")
		assetRoute.Use(middleware.UserAuth())
		{
			assetRoute.GET("/", controller.ListUserAssets)
			assetRoute.POST("/", middleware.CriticalRateLimit(), controller.UploadUserAsset)
			assetRoute.DELETE("/:id", middleware.CriticalRateLimit(), controller.DeleteUserAsset)
		}

		scriptApiRoute := apiRouter.Group("/script-api")
		scriptApiRoute.Use(middleware.CORS(), middleware.TokenAuthReadOnly())
		{
			scriptApiRoute.GET("/scripts/published", controller.ApiListPublishedScripts)
			scriptApiRoute.GET("/scripts/mine", controller.ApiListMyScripts)
			scriptApiRoute.GET("/scripts/:id", controller.ApiGetMyScript)
			scriptApiRoute.GET("/square/:id/code", controller.ApiGetPublishedScriptCode)

			scriptApiRoute.POST("/scripts", middleware.CriticalRateLimit(), controller.ApiSaveMyScriptDraft)
			scriptApiRoute.PUT("/scripts/:id", middleware.CriticalRateLimit(), controller.ApiSaveMyScriptDraft)
		}

		// Provider device identity and node/capability management (Stage C).
		deviceRoute := apiRouter.Group("/devices")
		{
			// Refresh only needs the refresh token, not a user session.
			deviceRoute.POST("/refresh", middleware.CriticalRateLimit(), controller.RefreshDeviceSession)

			deviceUserRoute := deviceRoute.Group("/")
			// Accept either a dashboard session or an API key (plugin uses the key).
			deviceUserRoute.Use(middleware.CORS(), middleware.TokenOrUserAuth())
			{
				deviceUserRoute.GET("/mine", controller.ListMyDevices)
				deviceUserRoute.POST("/challenge", middleware.CriticalRateLimit(), controller.CreateDeviceChallenge)
				deviceUserRoute.POST("/activate", middleware.CriticalRateLimit(), controller.ActivateDevice)
				deviceUserRoute.DELETE("/:deviceId", controller.RevokeMyDevice)
				deviceUserRoute.DELETE("/:deviceId/purge", controller.DeleteMyDevice)
			}
		}

		// Provider control channel (WSS). Auth is via the device token carried in
		// Sec-WebSocket-Protocol (browsers can't set WS headers), so it is NOT
		// behind DeviceOrUserAuth — the handler authenticates itself.
		apiRouter.GET("/nodes/control", controller.HandleNodeControl)

		// E2EE data-plane relay (WSS). Forwards opaque ciphertext frames between
		// the two sides of a task; self-authenticating via device token.
		apiRouter.GET("/relay", controller.HandleDataPlaneRelay)

		nodeRoute := apiRouter.Group("/nodes")
		nodeRoute.Use(middleware.CORS(), middleware.DeviceOrUserAuth())
		{
			nodeRoute.POST("", controller.RegisterNode)
			nodeRoute.GET("/mine", controller.ListMyNodes)
			nodeRoute.GET("/capability-stats", controller.ListMyCapabilityStats)
			nodeRoute.GET("/task-attempts", controller.ListMyTaskAttempts)
			// Provider group: the caller's own group (get-or-create) and a name
			// search used by clients to filter offers by provider.
			nodeRoute.GET("/provider-group/mine", controller.GetMyProviderGroup)
			nodeRoute.GET("/provider-groups/search", controller.SearchProviderGroups)
			nodeRoute.DELETE("/:id", controller.DeleteMyNode)
			nodeRoute.POST("/:id/enabled", controller.SetNodeEnabled)
			nodeRoute.POST("/:id/heartbeat", controller.NodeHeartbeat)
			nodeRoute.POST("/:id/balance-check", controller.ReportBalanceCheck)
			nodeRoute.POST("/:id/balance-check/request", controller.RequestBalanceCheck)
			nodeRoute.GET("/:id/balance-checks", controller.ListBalanceChecks)
			nodeRoute.GET("/:id/capabilities", controller.ListCapabilities)
			nodeRoute.POST("/:id/capabilities/:scriptId/test", controller.CreateCapabilityTest)
			nodeRoute.PUT("/:id/capabilities/:scriptId", controller.EnableCapability)
			nodeRoute.DELETE("/:id/capabilities/:scriptId", controller.DisableCapability)
		}

		// Client orders: quote, create (idempotent), status, cancel (Stage D).
		orderRoute := apiRouter.Group("/orders")
		orderRoute.Use(middleware.CORS(), middleware.DeviceOrUserAuth())
		{
			orderRoute.POST("/media/probe", controller.ProbeMediaURL)
			orderRoute.POST("/quote", controller.QuoteOrder)
			orderRoute.POST("", middleware.CriticalRateLimit(), controller.CreateOrder)
			orderRoute.GET("/:id", controller.GetOrder)
			orderRoute.POST("/:id/redispatch", middleware.CriticalRateLimit(), controller.RedispatchOrder)
			orderRoute.POST("/:id/cancel", controller.CancelOrder)
			// Signed dual-party receipts drive settlement/dispute (Stage E).
			orderRoute.POST("/:id/receipts", controller.SubmitReceipt)
		}

		// Ledger: balances, earnings, and recharging the marketplace available
		// balance from the caller's main wallet quota (Stage F).
		ledgerRoute := apiRouter.Group("/ledger")
		ledgerRoute.Use(middleware.CORS(), middleware.DeviceOrUserAuth())
		{
			ledgerRoute.GET("/balances", controller.GetMyLedgerBalances)
			ledgerRoute.GET("/earnings", controller.GetMyEarnings)
			ledgerRoute.POST("/recharge", middleware.CriticalRateLimit(), controller.RechargeAvailable)
			ledgerRoute.POST("/withdraw", middleware.CriticalRateLimit(), controller.WithdrawEarnings)
			ledgerRoute.POST("/withdraw-available", middleware.CriticalRateLimit(), controller.WithdrawAvailable)
		}

		// Payment: deposit address, fee estimate, withdrawal (Stage G).
		paymentRoute := apiRouter.Group("/payment")
		paymentRoute.Use(middleware.CORS(), middleware.DeviceOrUserAuth())
		{
			paymentRoute.POST("/deposit-address", controller.CreateDepositAddress)
			paymentRoute.POST("/withdrawals/estimate", controller.EstimateWithdrawalFee)
			paymentRoute.POST("/withdrawals", middleware.CriticalRateLimit(), controller.RequestWithdrawal)
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
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/search", controller.SearchRedemptions)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", controller.AddRedemption)
			redemptionRoute.PUT("/", controller.UpdateRedemption)
			redemptionRoute.DELETE("/invalid", controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)
		}
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		// Legacy synchronous direct-delete route used only by the classic frontend.
		// TODO: remove once the classic frontend is removed; the default frontend uses /system-task/log-cleanup.
		logRoute.DELETE("/", middleware.RootAuth(), controller.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), middleware.SearchRateLimit(), controller.SearchUserLogs)

		systemTaskRoute := apiRouter.Group("/system-task")
		systemTaskRoute.Use(middleware.RootAuth())
		{
			systemTaskRoute.POST("/log-cleanup", controller.CreateLogCleanupSystemTask)
			systemTaskRoute.GET("/list", controller.ListSystemTasks)
			systemTaskRoute.GET("/current", controller.GetCurrentSystemTask)
			systemTaskRoute.GET("/:task_id", controller.GetSystemTask)
		}
		systemInfoRoute := apiRouter.Group("/system-info")
		systemInfoRoute.Use(middleware.RootAuth())
		{
			systemInfoRoute.GET("/instances", controller.ListSystemInstances)
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
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("/self/stat", middleware.UserAuth(), controller.GetUserTaskStat)
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)
			taskRoute.GET("/stat", middleware.AdminAuth(), controller.GetTaskStat)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
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
			modelsRoute.GET("/upstream-metadata", controller.GetUpstreamModelMetadata)
			modelsRoute.GET("/missing", controller.GetMissingModels)
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
