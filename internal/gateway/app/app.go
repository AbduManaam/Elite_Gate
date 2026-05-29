package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"elitegate/internal/config"
	gatewayRouter "elitegate/internal/gateway"
	"elitegate/internal/gateway/runtime"
	gateway "elitegate/internal/gateway/server"
	"elitegate/internal/observability"
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

	// Connect to PostgreSQL using Config struct
	db, err := storage.NewPostgres(logger, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Connect to Redis using Config struct
	rdb, err := storage.NewRedis(cfg.Redis)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to connect to Redis at startup; falling back to in-memory rate limiting")
		rdb = nil
	}

	// Setup dynamic route loader
	routeRepo := storage.NewRouteRepo(db)
	reloadInterval, err := time.ParseDuration(cfg.Server.RouteReloadInterval)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to parse route reload interval; defaulting to 10s")
		reloadInterval = 10 * time.Second
	}
	loader := runtime.NewLoader(routeRepo, logger, reloadInterval)
	if err := loader.Start(context.Background()); err != nil {
		if rdb != nil {
			_ = rdb.Close()
		}
		_ = db.Close()
		return nil, fmt.Errorf("failed to start route loader: %w", err)
	}

	// Injected router configurations
	router, err := gatewayRouter.NewRouter(logger, rdb, cfg, loader)
	if err != nil {
		if rdb != nil {
			_ = rdb.Close()
		}
		_ = db.Close()
		return nil, fmt.Errorf("failed to build router: %w", err)
	}

	// Resolve dynamic server port
	port := cfg.Server.GatewayPort
	if port == "" {
		port = cfg.Server.Port
	}
	if port != "" && port[0] != ':' {
		port = ":" + port
	}

	server, err := gateway.NewServer(port, router, logger, cfg.Server, loader)
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
