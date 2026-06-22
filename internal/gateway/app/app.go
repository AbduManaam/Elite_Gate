package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

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
	DB          *sql.DB
	Redis       *redis.Client
	RouteLoader *runtime.Loader
}

func StartApp(cfg *config.Config) (*App, error) {
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logger := observability.NewLogger(cfg.Log)
	logger.Info().Msg("elitegate gateway starting...")

	// Connect to PostgreSQL
	db, err := storage.NewPostgres(logger, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Connect to Redis
	rdb, err := storage.NewRedis(cfg.Redis)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to connect to Redis at startup; falling back to in-memory rate limiting")
		rdb = nil
	}

	// Setup dynamic route loader to refresh routes and upstream pools on reload.
	routeRepo := storage.NewRouteRepo(db, logger)
	upstreamRepo := storage.NewUpstreamRepo(db, logger)
	upstreamTargetRepo := storage.NewUpstreamTargetRepo(db, logger)
	reloadInterval, err := time.ParseDuration(cfg.Server.RouteReloadInterval)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to parse route reload interval; defaulting to 10s")
		reloadInterval = 10 * time.Second
	}
	loader := runtime.NewLoader(routeRepo, upstreamTargetRepo, upstreamRepo, logger, reloadInterval)

	loaderCtx := context.Background()
	if cfg.Server.ProjectID != "" {
		if projectUUID, err := uuid.Parse(cfg.Server.ProjectID); err == nil {
			tc := storage.TenantContext{ProjectID: projectUUID}
			loaderCtx = storage.WithTenantContext(loaderCtx, tc)
			logger.Info().Str("project_id", cfg.Server.ProjectID).Msg("Gateway running in isolated single-project mode")
		} else {
			logger.Error().Err(err).Str("project_id", cfg.Server.ProjectID).Msg("Invalid PROJECT_ID format; running globally")
		}
	}

	// ── Health Checker ────────────────────────────────────────────────────
	// Created before loader.Start() and wired into the loader so the very
	// first reload registers every backend target (from every upstream's
	// LB pool) for probing — not just the legacy single-target case.
	hc := health.New(
		10*time.Second, // probe every 10 seconds
		"/health",      // health endpoint path on each upstream
		3*time.Second,  // per-probe HTTP timeout
		logger,
	)
	loader.SetHealthChecker(hc)
	// ─────────────────────────────────────────────────────────────────────

	if err := loader.Start(loaderCtx); err != nil {
		if rdb != nil {
			_ = rdb.Close()
		}
		_ = db.Close()
		return nil, fmt.Errorf("failed to start route loader: %w", err)
	}

	hc.Start(loaderCtx) // stops automatically when loaderCtx is cancelled (shutdown)

	// Injected shared security configurations
	jwtValidator := auth.NewJWTValidator(cfg.Auth.JWTSecret)
	apiKeyRepo := storage.NewApiKeyRepo(db)
	keyStore := auth.NewRedisKeyStore(rdb, apiKeyRepo)
	authMiddleware := middleware.NewAuthMiddleware(jwtValidator, keyStore, &logger)

	rpm := cfg.RateLimit.RequestsPerMinute
	memFallback := ratelimit.NewMemoryLimiter(rpm)
	memFallback.StartCleanup(loaderCtx, time.Minute)

	limiter := ratelimit.NewRedisLimiter(rdb, rpm, memFallback)

	router, err := gatewayRouter.NewRouter(logger, db, rdb, cfg, loader, authMiddleware, limiter, hc)
	if err != nil {
		if rdb != nil {
			_ = rdb.Close()
		}
		_ = db.Close()
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
		_ = db.Close()
		return nil, fmt.Errorf("failed to create gateway server: %w", err)
	}

	return &App{
		Logger:      logger,
		Config:      cfg,
		Server:      server,
		DB:          db,
		Redis:       rdb,
		RouteLoader: loader,
	}, nil
}
