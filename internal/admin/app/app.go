package app

import (
	"fmt"
	"os"

	"edgecore/internal/admin"
	adminserver "edgecore/internal/admin/server"
	"edgecore/internal/config"
	"edgecore/internal/observability"

	"github.com/rs/zerolog"
)

type App struct {
	Logger zerolog.Logger
	Server *adminserver.Server
}

func StartApp(cfg *config.Config) (*App, error) {
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logCfg := cfg.Log
	logCfg.File = "logs/admin.log"
	logger := observability.NewServiceLogger(logCfg, "edgecore-admin")
	logger.Info().Msg("edgecore admin starting...")

	router := admin.NewRouter(logger)
	server := adminserver.NewServer(adminPort(), router, logger)

	return &App{
		Logger: logger,
		Server: server,
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
