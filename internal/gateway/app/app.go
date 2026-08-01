package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"elitegate/helper"
	"elitegate/internal/auth"
	"elitegate/internal/config"
	gatewayRouter "elitegate/internal/gateway"
	"elitegate/internal/gateway/health"
	"elitegate/internal/gateway/middleware"
	"elitegate/internal/gateway/runtime"
	gateway "elitegate/internal/gateway/server"
	"elitegate/internal/observability"
	"elitegate/internal/ratelimit"
	"elitegate/internal/storage"
)

type App struct {
	Logger      zerolog.Logger
	Config      *config.Config
	Server      *gateway.Server
	Redis       *redis.Client
	RouteLoader *runtime.Loader
}

func StartApp(cfg *config.Config) (*App, error) {
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logger := observability.NewLogger(cfg.Log)
	logger.Info().Msg("elitegate gateway starting...")

	// Connect to Redis
	rdb, err := storage.NewRedis(cfg.Redis)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to connect to Redis at startup; falling back to in-memory rate limiting")
		rdb = nil
	}

	if cfg.Server.ProjectID == "" {
		return nil, fmt.Errorf("PROJECT_ID is required when running against a restricted gateway DB role — refusing to start in unscoped/global mode")
	}
	_, err = uuid.Parse(cfg.Server.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("invalid PROJECT_ID %q: %w", cfg.Server.ProjectID, err)
	}
	logger.Info().Str("project_id", cfg.Server.ProjectID).Msg("Gateway running in isolated single-project mode")

	reloadInterval, err := time.ParseDuration(cfg.Server.RouteReloadInterval)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to parse route reload interval; defaulting to 10s")
		reloadInterval = 10 * time.Second
	}

	// The gateway authenticates the /internal/v1/projects/:id/sync endpoint with a
	// project-scoped HMAC token. GATEWAY_SYNC_TOKEN (if set) carries that value
	// explicitly (used by provisioned containers via docker.go). If absent we derive
	// it here — this handles the static dev container in docker-compose whose
	// JWT_SECRET is the master secret rather than a derived one.
	gatewaySyncToken := cfg.Auth.GatewaySyncToken
	if gatewaySyncToken == "" {
		gatewaySyncToken = helper.DeriveTenantJWTSecret(cfg.Auth.JWTSecret, cfg.Server.ProjectID)
		logger.Debug().Msg("GATEWAY_SYNC_TOKEN not set, derived from JWT_SECRET + PROJECT_ID")
	}

	// Injected ControlPlaneClient to sync snapshot routes, upstreams, policies, and api keys
	controlClient := runtime.NewControlPlaneClient(
		cfg.Server.AdminAPIURL,
		cfg.Server.ProjectID,
		gatewaySyncToken,
		logger,
	)

	loader := runtime.NewLoader(controlClient, rdb, logger, reloadInterval)

	// ── Health Checker ────────────────────────────────────────────────────
	hc := health.New(
		10*time.Second, // probe every 10 seconds
		3*time.Second,  // per-probe HTTP timeout
		logger,
	)
	loader.SetHealthChecker(hc)
	// ─────────────────────────────────────────────────────────────────────

	gatewayCtx := context.Background()

	if err := loader.Start(gatewayCtx); err != nil {
		if rdb != nil {
			_ = rdb.Close()
		}
		return nil, fmt.Errorf("failed to start route loader: %w", err)
	}

	hc.Start(gatewayCtx) // stops automatically when gatewayCtx is cancelled (shutdown)

	// Injected shared security configurations
	jwtValidator := auth.NewJWTValidator(cfg.Auth.JWTSecret)
	// db is gone; RedisKeyStore's cache is kept warm by the loader instead:
	keyStore := auth.NewRedisKeyStore(rdb, nil)
	loader.SetKeyStore(keyStore)
	authMiddleware := middleware.NewAuthMiddleware(jwtValidator, keyStore, &logger)

	rpm := cfg.RateLimit.RequestsPerMinute
	memFallback := ratelimit.NewMemoryLimiter(rpm)
	memFallback.StartCleanup(gatewayCtx, time.Minute)

	limiter := ratelimit.NewRedisLimiter(rdb, rpm, memFallback)

	router, err := gatewayRouter.NewRouter(logger, rdb, cfg, loader, authMiddleware, limiter, hc)
	if err != nil {
		if rdb != nil {
			_ = rdb.Close()
		}
		return nil, fmt.Errorf("failed to build router: %w", err)
	}

	// gRPC Interceptors setup
	grpcInterceptors := gateway.NewGRPCSecurityInterceptors(loader, authMiddleware, limiter, cfg.Server.TrustProxy, logger)

	// Resolve dynamic server port
	port := cfg.Server.GatewayPort
	if port == "" {
		port = cfg.Server.Port
	}
	if port != "" && port[0] != ':' {
		port = ":" + port
	}

	server, err := gateway.NewServer(port, router, logger, cfg.Server, loader, grpcInterceptors, hc)
	if err != nil {
		if rdb != nil {
			_ = rdb.Close()
		}
		return nil, fmt.Errorf("failed to create gateway server: %w", err)
	}

	return &App{
		Logger:      logger,
		Config:      cfg,
		Server:      server,
		Redis:       rdb,
		RouteLoader: loader,
	}, nil
}
