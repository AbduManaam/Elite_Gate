package app

import (
	"database/sql"
	"fmt"
	"os"

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
}

func StartApp(cfg *config.Config) (*App, error) {

	// Create logs directory 
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Build logger 
	logger := observability.NewLogger(cfg.Log)
	logger.Info().Msg("elitegate gateway starting...")

	//  Connect to database 
	db, err := storage.NewPostgres(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Build router 
	router, err := gatewayRouter.NewRouter(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build router: %w", err)
	}

	// Build server 
	server := gateway.NewServer(cfg.Server.Port, router, logger)

	return &App{
		Logger: logger,
		Config: cfg,
		Server: server,
		DB:     db,
	}, nil
}
