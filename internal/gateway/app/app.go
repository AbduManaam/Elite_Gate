package app

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"elitegate/internal/config"
	gatewayRouter "elitegate/internal/gateway"
	gateway "elitegate/internal/gateway/server"
	"elitegate/internal/observability"
	"elitegate/internal/storage"
)

type App struct {
	Logger zerolog.Logger
	Config *config.Config
	Server *gateway.Server
	DB     *sql.DB
	Redis  *redis.Client
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

	// Injected router configurations
	router, err := gatewayRouter.NewRouter(logger, rdb, cfg)
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

	server, err := gateway.NewServer(port, router, logger, cfg.Server)
	if err != nil {
		if rdb != nil {
			_ = rdb.Close()
		}
		_ = db.Close()
		return nil, fmt.Errorf("failed to create gateway server: %w", err)
	}

	return &App{
		Logger: logger,
		Config: cfg,
		Server: server,
		DB:     db,
		Redis:  rdb,
	}, nil
}
