package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"elitegate/internal/admin/handler"
	"elitegate/internal/admin/middleware"
	"elitegate/internal/auth"
	"elitegate/internal/config"
	"elitegate/internal/container"
	"elitegate/internal/ipfilter"
	"elitegate/internal/ratelimit"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

func NewRouter(logger zerolog.Logger, db *sql.DB, cfg *config.Config, containerMgr container.ContainerManager) (http.Handler, error) {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
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
	authHandler := handler.NewAuthHandler(authRepo, adminTokens, loginLimiter, logger)

	// Repositories initialized
	routeRepo := storage.NewRouteRepo(db, logger)
	upstreamRepo := storage.NewUpstreamRepo(db, logger)
	upstreamTargetRepo := storage.NewUpstreamTargetRepo(db, logger)
	policyRepo := storage.NewPolicyRepo(db)
	projectRepo := storage.NewProjectRepo(db, logger)
	apiKeyRepo := storage.NewApiKeyRepo(db)
	auditLogRepo := storage.NewAuditLogRepo(db, logger)
	gatewayRepo := storage.NewGatewayRepo(db)

	// Handlers initialized
	routeHandler := handler.NewRouteHandler(routeRepo, logger)
	upstreamHandler := handler.NewUpstreamHandler(upstreamRepo, logger)
	upstreamTargetHandler := handler.NewUpstreamTargetHandler(upstreamTargetRepo, logger)
	policyHandler := handler.NewPolicyHandler(policyRepo, routeRepo, logger)
	projectHandler := handler.NewProjectHandler(projectRepo, logger)
	apiKeyHandler := handler.NewApiKeyHandler(apiKeyRepo, logger)
	auditLogHandler := handler.NewAuditLogHandler(auditLogRepo, logger)
	drainTimeout, err := time.ParseDuration(cfg.Server.DrainTimeout)
	if err != nil {
		return nil, fmt.Errorf("parse server.drain_timeout: %w", err)
	}
	gatewayHandler := handler.NewGatewayHandler(gatewayRepo, containerMgr, drainTimeout)
	syncHandler := handler.NewSyncHandler(gatewayRepo, logger)
	platformHandler := handler.NewPlatformHandler(
		projectRepo, gatewayRepo, authRepo, containerMgr, syncHandler, logger,
	)

	var ipAllowlist gin.HandlerFunc
	if len(cfg.Server.AdminIPAllowlist) > 0 {
		checker, err := ipfilter.NewIPChecker(cfg.Server.AdminIPAllowlist)
		if err != nil {
			return nil, fmt.Errorf("invalid IP allowlist configuration: %w", err)
		}
		ipAllowlist = middleware.AdminIPAllowlist(checker, cfg.Server.TrustProxy)
	}

	adminGroup := r.Group("/admin")
	if ipAllowlist != nil {
		adminGroup.Use(ipAllowlist)
	}

	adminGroup.POST("/login", authHandler.Login)
	adminGroup.POST("/refresh", authHandler.Refresh)
	adminGroup.POST("/logout", authHandler.Logout)

	// ── Public bootstrap registration ─────────────────────────────────
	// Only works when 0 admin users exist in the database.
	// After the first admin is created, returns 403 Forbidden.
	adminGroup.POST("/register", authHandler.Register)

	// ── Permanent public self-service tenant signup ───────────────────────
	// Any company can call this to self-onboard. No token, no super-admin needed.
	// This is the PRIMARY onboarding path for all tenants on the SaaS platform.
	adminGroup.POST("/signup", authHandler.Signup)

	v1 := adminGroup.Group("/v1")
	v1.Use(middleware.AdminAuth(adminTokens))
	{
		membershipRepo := storage.NewMembershipRepo(db, logger)
		membershipHandler := handler.NewMembershipHandler(membershipRepo, logger)
		userLookupLimiter := ratelimit.NewMemoryLimiter(20) // 20 lookups/min per caller
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
			platform.PATCH("/projects/:projectId/suspend", platformHandler.SuspendTenant)
			platform.PATCH("/projects/:projectId/reactivate", platformHandler.ReactivateTenant)
			platform.POST("/gateways/:gatewayId/restart", platformHandler.RestartGateway)
			platform.POST("/gateways/:gatewayId/force-decommission", platformHandler.ForceDecommission)
		}

		projectGroup := v1.Group("/projects/:projectId")
		projectGroup.Use(middleware.ProjectScope(membershipRepo, projectRepo))
		{
			routes := projectGroup.Group("/routes")
			{
				routes.GET("", middleware.RBAC(middleware.RoleViewer), routeHandler.List)
				routes.POST("", middleware.RBAC(middleware.RoleEditor), routeHandler.Create)
				routes.PUT("/:id", middleware.RBAC(middleware.RoleEditor), routeHandler.Update)
				routes.PATCH("/:id/disable", middleware.RBAC(middleware.RoleEditor), routeHandler.Disable)
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
			projectGroup.POST("/reload", middleware.RBAC(middleware.RoleEditor), syncHandler.ReloadProject)
		}

		admins := v1.Group("/admins")
		{
			// SuperAdminOnly: this endpoint is a PLATFORM-OPERATOR SUPPORT TOOL.
			// Normal tenants onboard via POST /admin/signup — they never hit this route.
			// Use this only for support escalations (e.g. manually provisioning an account
			// on a tenant's behalf). Future operator features reuse the same middleware.
			admins.POST("", middleware.SuperAdminOnly(authRepo, logger), authHandler.Register)
		}
	}

	logger.Debug().Msg("admin router configured")
	return r, nil
}
