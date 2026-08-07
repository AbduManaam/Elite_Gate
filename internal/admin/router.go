package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"elitegate/internal/admin/handler"
	"elitegate/internal/admin/middleware"
	"elitegate/internal/admin/service"
	"elitegate/internal/auth"
	"elitegate/internal/config"
	"elitegate/internal/container"
	"elitegate/internal/ipfilter"
	"elitegate/internal/mailer"
	"elitegate/internal/promclient"
	"elitegate/internal/ratelimit"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

func NewRouter(logger zerolog.Logger, db *sql.DB, cfg *config.Config, containerMgr container.ContainerManager) (http.Handler, error) {
	gin.SetMode(gin.ReleaseMode)

	projectRepo := storage.NewProjectRepo(db, logger)
	originCache := middleware.NewOriginCache(projectRepo, 30*time.Second)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(middleware.DynamicCORS(cfg.Server.AllowedOrigins, originCache))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "admin-api",
		})
	})

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "admin-api",
		})
	})

	//  Metrics for Prometheus
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
		})
	})

	adminTokens, err := auth.NewAdminTokenManager(cfg.Auth.JWTSecret, "elitegate-admin")
	if err != nil {
		return nil, fmt.Errorf("create admin token manager: %w", err)
	}
	authRepo := storage.NewAdminAuthRepo(db)
	loginLimiter := middleware.NewLoginRateLimiter(5, time.Minute)

	var mailClient mailer.Mailer
	if cfg.Mail.Enabled {
		if cfg.AppEnv == "production" && cfg.Mail.Host == "" {
			return nil, errors.New("SMTP configuration is required in production when mail is enabled")
		}
		if cfg.Mail.Host != "" {
			smtpClient, err := mailer.NewSMTPMailer(mailer.SMTPConfig{
				Host:       cfg.Mail.Host,
				Port:       cfg.Mail.Port,
				Username:   cfg.Mail.Username,
				Password:   cfg.Mail.Password,
				FromEmail:  cfg.Mail.FromEmail,
				FromName:   cfg.Mail.FromName,
				TLSMode:    cfg.Mail.TLSMode,
				Production: cfg.AppEnv == "production",
			})
			if err != nil {
				return nil, fmt.Errorf("initialize SMTP mailer: %w", err)
			}
			mailClient = smtpClient
		}
	}

	authHandler := handler.NewAuthHandler(
		authRepo,
		adminTokens,
		loginLimiter,
		mailClient,
		cfg.Mail.PasswordResetURL,
		logger,
		cfg.AppEnv == "production",
	)

	// Enable Google OAuth if it is configured.
	if cfg.GoogleOAuth.ClientID != "" {
		oauthState := auth.NewOAuthStateManager(cfg.GoogleOAuth.StateSecret)

		googleOAuth := auth.NewGoogleOAuth(
			cfg.GoogleOAuth.ClientID,
			cfg.GoogleOAuth.ClientSecret,
			cfg.GoogleOAuth.RedirectURL,
		)

		authHandler.EnableGoogleOAuth(
			oauthState,
			googleOAuth,
			cfg.GoogleOAuth.FrontendURL,
		)
	}

	// Repositories initialized
	routeRepo := storage.NewRouteRepo(db, logger)
	upstreamRepo := storage.NewUpstreamRepo(db, logger)
	upstreamTargetRepo := storage.NewUpstreamTargetRepo(db, logger)
	policyRepo := storage.NewPolicyRepo(db)
	apiKeyRepo := storage.NewApiKeyRepo(db)
	auditLogRepo := storage.NewAuditLogRepo(db, logger)
	gatewayRepo := storage.NewGatewayRepo(db)
	customDomainRepo := storage.NewCustomDomainRepo(db, logger)

	// Services initialized
	auditSvc := service.NewAuditService(auditLogRepo, logger)
	projectSvc := service.NewProjectService(projectRepo, originCache, logger)
	routeSvc := service.NewRouteService(routeRepo, logger)
	upstreamSvc := service.NewUpstreamService(upstreamRepo, upstreamTargetRepo, logger)
	policySvc := service.NewPolicyService(policyRepo, routeRepo, logger)
	apiKeySvc := service.NewApiKeyService(apiKeyRepo, logger)
	customDomainSvc := service.NewCustomDomainServiceWithAutomation(
		customDomainRepo,
		net.DefaultResolver,
		cfg.Server.GatewayHostPublic,
		cfg.AWS.AutomationEnabled,
		logger,
	)

	membershipRepo := storage.NewMembershipRepo(db, logger)
	membershipSvc := service.NewMembershipService(membershipRepo, logger)
	membershipHandler := handler.NewMembershipHandler(membershipSvc, logger, auditSvc)
	platformSvc := service.NewPlatformService(projectRepo, gatewayRepo, authRepo, logger)

	// Handlers initialized
	routeHandler := handler.NewRouteHandler(routeSvc, logger, auditSvc)
	upstreamHandler := handler.NewUpstreamHandler(upstreamSvc, logger, auditSvc)
	upstreamTargetHandler := handler.NewUpstreamTargetHandler(upstreamSvc, logger, auditSvc)
	policyHandler := handler.NewPolicyHandler(policySvc, logger, auditSvc)
	projectHandler := handler.NewProjectHandler(projectSvc, logger)
	apiKeyHandler := handler.NewApiKeyHandler(apiKeySvc, logger, auditSvc)
	auditLogHandler := handler.NewAuditLogHandler(auditLogRepo, logger)

	customDomainHandler := handler.NewCustomDomainHandler(
		customDomainSvc,
		logger,
		auditSvc,
	)
	drainTimeout, err := time.ParseDuration(cfg.Server.DrainTimeout)
	if err != nil {
		return nil, fmt.Errorf("parse server.drain_timeout: %w", err)
	}
	gatewayHandler := handler.NewGatewayHandler(gatewayRepo, containerMgr, drainTimeout)
	syncHandler := handler.NewSyncHandler(gatewayRepo, logger)
	platformHandler := handler.NewPlatformHandler(
		platformSvc, gatewayRepo, containerMgr, syncHandler, logger,
	)
	promClient := promclient.NewClient(cfg.Server.PrometheusURL, 5*time.Second)
	metricsCacheTTL, err := time.ParseDuration(cfg.Server.MetricsCacheTTL)
	if err != nil {
		return nil, fmt.Errorf("parse server.metrics_cache_ttl: %w", err)
	}
	metricsSvc := service.NewMetricsService(promClient, metricsCacheTTL)
	metricsHandler := handler.NewMetricsHandler(metricsSvc, gatewayRepo, logger)

	var ipAllowlist gin.HandlerFunc
	if len(cfg.Server.AdminIPAllowlist) > 0 {
		checker, err := ipfilter.NewIPChecker(cfg.Server.AdminIPAllowlist)
		if err != nil {
			return nil, fmt.Errorf("invalid IP allowlist configuration: %w", err)
		}
		ipAllowlist = middleware.AdminIPAllowlist(checker, cfg.Server.TrustProxy)
	}

	// Read rate limits from config
	loginRPM := cfg.RateLimit.Auth.LoginRPM
	refreshRPM := cfg.RateLimit.Auth.RefreshRPM
	oauthRPM := cfg.RateLimit.Auth.OAuthCallbackRPM
	signupRPM := cfg.RateLimit.Auth.SignupRPM
	forgotPasswordRPM := cfg.RateLimit.Auth.ForgotPasswordRPM
	resetPasswordRPM := cfg.RateLimit.Auth.ResetPasswordRPM
	trustProxy := cfg.Server.TrustProxyHeaders

	// Instantiate memory limiters for public endpoints
	loginIPLimiter := ratelimit.NewMemoryLimiter(loginRPM)
	loginIPLimiter.StartCleanup(context.Background(), time.Minute)

	refreshIPLimiter := ratelimit.NewMemoryLimiter(refreshRPM)
	refreshIPLimiter.StartCleanup(context.Background(), time.Minute)

	signupIPLimiter := ratelimit.NewMemoryLimiter(signupRPM)
	signupIPLimiter.StartCleanup(context.Background(), time.Minute)

	forgotIPLimiter := ratelimit.NewMemoryLimiter(forgotPasswordRPM)
	forgotIPLimiter.StartCleanup(context.Background(), time.Minute)

	resetIPLimiter := ratelimit.NewMemoryLimiter(resetPasswordRPM)
	resetIPLimiter.StartCleanup(context.Background(), time.Minute)

	adminGroup := r.Group("/admin")
	if ipAllowlist != nil {
		adminGroup.Use(ipAllowlist)
	}

	adminGroup.POST("/login", middleware.IPRateLimit(loginIPLimiter, loginRPM, "login", trustProxy), authHandler.Login)
	adminGroup.POST("/refresh", middleware.IPRateLimit(refreshIPLimiter, refreshRPM, "refresh", trustProxy), authHandler.Refresh)
	adminGroup.POST("/logout", authHandler.Logout)
	adminGroup.POST("/forgot-password", middleware.IPRateLimit(forgotIPLimiter, forgotPasswordRPM, "forgot-password", trustProxy), authHandler.ForgotPassword)
	adminGroup.POST("/reset-password", middleware.IPRateLimit(resetIPLimiter, resetPasswordRPM, "reset-password", trustProxy), authHandler.ResetPassword)

	// Public bootstrap registration.
	// Disabled after the first admin account is created.
	adminGroup.POST("/register", middleware.IPRateLimit(signupIPLimiter, signupRPM, "register", trustProxy), authHandler.Register)

	// Public tenant signup.
	// Used by companies to create their own account.
	adminGroup.POST("/signup", middleware.IPRateLimit(signupIPLimiter, signupRPM, "signup", trustProxy), authHandler.Signup)

	// Public Google OAuth endpoints.
	// Used to start the Google sign-in flow and handle Google's callback.
	if cfg.GoogleOAuth.ClientID != "" {
		oauthCallbackLimiter := ratelimit.NewMemoryLimiter(oauthRPM)
		oauthCallbackLimiter.StartCleanup(context.Background(), time.Minute)

		adminGroup.GET("/google/login", authHandler.GoogleLogin)
		adminGroup.GET("/google/callback", middleware.IPRateLimit(oauthCallbackLimiter, oauthRPM, "google-callback", trustProxy), authHandler.GoogleCallback)
	}

	v1 := adminGroup.Group("/v1")
	v1.Use(middleware.AdminAuth(adminTokens))
	v1.GET("/me", authHandler.Me)
	{
		userLookupLimiter := ratelimit.NewMemoryLimiter(20) // 20 lookups/min per caller
		userLookupLimiter.StartCleanup(context.Background(), time.Minute)
		userLookupLimit := middleware.UserLookupRateLimit(userLookupLimiter, 20)

		// Project Management
		v1.POST("/projects", projectHandler.Create)
		v1.GET("/projects", projectHandler.List)
		v1.PUT("/projects/:projectId", projectHandler.Update)
		v1.DELETE("/projects/:projectId", projectHandler.Delete)
		v1.POST("/reload", middleware.SuperAdminOnly(authRepo, logger), syncHandler.Reload)
		v1.GET("/gateways", gatewayHandler.ListAllForAdmin)

		// ── Platform-operator-only routes ───────────────────────────────────
		// Cross-tenant. SuperAdminOnly gates every route in this group —
		// there is no per-handler authorization check, the gate is here, once.
		platform := v1.Group("/platform")
		platform.Use(middleware.SuperAdminOnly(authRepo, logger))
		{
			platform.GET("/projects", platformHandler.ListTenants)
			platform.DELETE("/projects/:projectId", platformHandler.DeleteTenant)
			platform.GET("/health", platformHandler.PlatformHealth)
			platform.GET("/metrics", platformHandler.PlatformMetrics)
			platform.GET("/metrics/system", metricsHandler.SystemMetrics)
			platform.GET("/metrics/system/range", metricsHandler.SystemMetricsRange)
			platform.PATCH("/projects/:projectId/suspend", platformHandler.SuspendTenant)
			platform.PATCH("/projects/:projectId/reactivate", platformHandler.ReactivateTenant)
			platform.POST("/gateways/:gatewayId/restart", platformHandler.RestartGateway)
			platform.POST("/gateways/:gatewayId/force-decommission", platformHandler.ForceDecommission)
		}

		projectGroup := v1.Group("/projects/:projectId")
		projectGroup.Use(middleware.ProjectScope(membershipRepo, projectRepo))
		{
			projectGroup.GET("/summary", middleware.RBAC(middleware.RoleViewer), projectHandler.GetSummary)
			projectGroup.PUT("/dashboard-origins", middleware.RBAC(middleware.RoleOwner), projectHandler.UpdateDashboardOrigins)

			routes := projectGroup.Group("/routes")
			{
				routes.GET("", middleware.RBAC(middleware.RoleViewer), routeHandler.List)
				routes.POST("", middleware.RBAC(middleware.RoleEditor), routeHandler.Create)
				routes.PUT("/:id", middleware.RBAC(middleware.RoleEditor), routeHandler.Update)
				routes.PATCH("/:id/disable", middleware.RBAC(middleware.RoleEditor), routeHandler.Disable)
				routes.PATCH("/:id/enable", middleware.RBAC(middleware.RoleEditor), routeHandler.Enable)
				routes.DELETE("/:id", middleware.RBAC(middleware.RoleEditor), routeHandler.Delete)
				routes.POST("/:id/policy", middleware.RBAC(middleware.RoleEditor), policyHandler.AssignPolicy)
				routes.DELETE("/:id/policy", middleware.RBAC(middleware.RoleEditor), policyHandler.RemovePolicy)
			}
			upstreams := projectGroup.Group("/upstreams")
			{
				upstreams.GET("", middleware.RBAC(middleware.RoleViewer), upstreamHandler.List)
				upstreams.POST("", middleware.RBAC(middleware.RoleEditor), upstreamHandler.Create)
				upstreams.GET("/:id/health", middleware.RBAC(middleware.RoleViewer), upstreamHandler.HealthCheck)
				upstreams.PUT("/:id", middleware.RBAC(middleware.RoleEditor), upstreamHandler.Update)
				upstreams.PATCH("/:id/disable", middleware.RBAC(middleware.RoleEditor), upstreamHandler.Disable)
				upstreams.DELETE("/:id", middleware.RBAC(middleware.RoleEditor), upstreamHandler.Delete)

				targets := upstreams.Group("/:id/targets")
				{
					targets.GET("", middleware.RBAC(middleware.RoleViewer), upstreamTargetHandler.List)
					targets.POST("", middleware.RBAC(middleware.RoleEditor), upstreamTargetHandler.Add)
					targets.DELETE("/:targetId", middleware.RBAC(middleware.RoleEditor), upstreamTargetHandler.Remove)
				}
			}
			policies := projectGroup.Group("/policies")
			{
				policies.GET("", middleware.RBAC(middleware.RoleViewer), policyHandler.List)
				policies.POST("", middleware.RBAC(middleware.RoleEditor), policyHandler.Create)
				policies.PUT("/:id", middleware.RBAC(middleware.RoleEditor), policyHandler.Update)
				policies.DELETE("/:id", middleware.RBAC(middleware.RoleOwner), policyHandler.Delete)
			}
			members := projectGroup.Group("/members")
			{
				members.GET("", middleware.RBAC(middleware.RoleViewer), membershipHandler.List)
				members.GET("/lookup", middleware.RBAC(middleware.RoleOwner), userLookupLimit, membershipHandler.LookupMemberByEmail)
				members.POST("", middleware.RBAC(middleware.RoleOwner), membershipHandler.AddMember)
				members.PUT("/:memberId", middleware.RBAC(middleware.RoleOwner), membershipHandler.ChangeRole)
				members.DELETE("/:memberId", middleware.RBAC(middleware.RoleOwner), membershipHandler.RemoveMember)
			}
			keys := projectGroup.Group("/keys")
			{
				keys.GET("", middleware.RBAC(middleware.RoleViewer), apiKeyHandler.List)
				keys.POST("", middleware.RBAC(middleware.RoleEditor), apiKeyHandler.Create)
				keys.POST("/:id/rotate", middleware.RBAC(middleware.RoleEditor), apiKeyHandler.Rotate)
				keys.DELETE("/:id", middleware.RBAC(middleware.RoleEditor), apiKeyHandler.Revoke)
			}
			customDomains := projectGroup.Group("/custom-domains")
			{
				customDomains.GET(
					"",
					middleware.RBAC(middleware.RoleViewer),
					customDomainHandler.List,
				)

				customDomains.GET(
					"/:domainId",
					middleware.RBAC(middleware.RoleViewer),
					customDomainHandler.Get,
				)

				customDomains.POST(
					"",
					middleware.RBAC(middleware.RoleOwner),
					customDomainHandler.Create,
				)

				customDomains.POST(
					"/:domainId/verify",
					middleware.RBAC(middleware.RoleOwner),
					customDomainHandler.Verify,
				)

				customDomains.DELETE(
					"/:domainId",
					middleware.RBAC(middleware.RoleOwner),
					customDomainHandler.Delete,
				)

				customDomains.POST(
					"/:domainId/check-routing",
					middleware.RBAC(middleware.RoleOwner),
					customDomainHandler.CheckRouting,
				)

				customDomains.POST(
					"/:domainId/activate",
					middleware.RBAC(middleware.RoleOwner),
					customDomainHandler.Activate,
				)

				customDomains.GET(
					"/:domainId/provisioning-status",
					middleware.RBAC(middleware.RoleViewer),
					customDomainHandler.GetProvisioningStatus,
				)

				customDomains.POST(
					"/:domainId/retry-provisioning",
					middleware.RBAC(middleware.RoleOwner),
					customDomainHandler.RetryProvisioning,
				)

				customDomains.POST(
					"/:domainId/retry-deprovisioning",
					middleware.RBAC(middleware.RoleOwner),
					customDomainHandler.RetryDeprovisioning,
				)
			}
			auditLogs := projectGroup.Group("/audit-logs")
			{
				auditLogs.GET("", middleware.RBAC(middleware.RoleViewer), auditLogHandler.List)
			}
			gateways := projectGroup.Group("/gateways")
			{
				gateways.GET("", middleware.RBAC(middleware.RoleViewer), gatewayHandler.List)
				gateways.POST("", middleware.RBAC(middleware.RoleEditor), gatewayHandler.Provision)
				gateways.DELETE("/:gatewayId", middleware.RBAC(middleware.RoleEditor), gatewayHandler.Decommission)
			}
			projectMetrics := projectGroup.Group("/metrics")
			{
				projectMetrics.GET("/query", middleware.RBAC(middleware.RoleViewer), metricsHandler.QueryInstant)
				projectMetrics.GET("/query-range", middleware.RBAC(middleware.RoleViewer), metricsHandler.QueryRange)
				projectMetrics.GET("/summary", middleware.RBAC(middleware.RoleViewer), metricsHandler.Summary)
				projectMetrics.GET("/system", middleware.RBAC(middleware.RoleViewer), metricsHandler.SystemMetrics)
				projectMetrics.GET("/system/range", middleware.RBAC(middleware.RoleViewer), metricsHandler.SystemMetricsRange)
			}
			projectGroup.POST("/reload", middleware.RBAC(middleware.RoleEditor), syncHandler.ReloadProject)
		}

		admins := v1.Group("/admins")
		{
			// SuperAdminOnly: Only platform admins use this API to help customers.
			// Regular users should use the normal signup endpoint.
			admins.POST("", middleware.SuperAdminOnly(authRepo, logger), authHandler.Register)
		}
	}

	tenantSync := handler.NewTenantSyncHandler(db, logger)
	internalGroup := r.Group("/internal/v1")
	internalGroup.Use(middleware.RequireGatewayToken(cfg.Auth.JWTSecret))
	{
		internalGroup.GET("/projects/:project_id/sync", tenantSync.GetTenantSnapshot)
	}

	logger.Debug().Msg("admin router configured")
	return r, nil
}
