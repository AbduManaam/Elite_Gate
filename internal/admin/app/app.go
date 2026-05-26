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

	// Connect to database
	db, err := storage.NewPostgres(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Read JWT secret
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	router := admin.NewRouter(logger, db, jwtSecret)
	server := adminserver.NewServer(adminPort(), router, logger)

	return &App{
		Logger: logger,
		Server: server,
		DB:     db,
	}, nil
}

func adminPort() string {
	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		return ":9090"
	}
	if port[0] == ':' {
		return port
	}
	return ":" + port
}
