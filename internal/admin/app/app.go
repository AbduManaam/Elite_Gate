package app

import (
	"database/sql"
	"fmt"
	"os"

	"elitegate/internal/admin"
	adminserver "elitegate/internal/admin/server"
	"elitegate/internal/config"
	"elitegate/internal/observability"
	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

type App struct {
	Logger zerolog.Logger
	Server *adminserver.Server
	DB     *sql.DB
}

func StartApp(cfg *config.Config) (*App, error) {
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logCfg := cfg.Log
	logCfg.File = "logs/admin.log"
	logger := observability.NewServiceLogger(logCfg, "elitegate-admin")
	logger.Info().Msg("elitegate admin starting...")

	// Connect to postgres using injected database configs
	db, err := storage.NewPostgres(logger, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	router, err := admin.NewRouter(logger, db, cfg.Auth.JWTSecret)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to build admin router: %w", err)
	}

	// Resolve dynamic admin server port
	port := cfg.Server.AdminPort
	if port == "" {
		port = ":9090"
	}
	if port != "" && port[0] != ':' {
		port = ":" + port
	}

	server, err := adminserver.NewServer(port, router, logger, cfg.Server)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create admin server: %w", err)
	}

	return &App{
		Logger: logger,
		Server: server,
		DB:     db,
	}, nil
}
