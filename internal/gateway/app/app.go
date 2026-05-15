package app

import (
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"edgecore/internal/config"
	"edgecore/internal/observability"
	gateway "edgecore/internal/gateway/server"
)

type App struct {
	Logger zerolog.Logger
	Config *config.Config
	Server *gateway.Server
}

func StartApp(cfg *config.Config) (*App, error) {

	// ── 1. Create logs directory ──────────────────────────────────────
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// ── 2. Build logger ───────────────────────────────────────────────
	logger := observability.NewLogger(cfg.Log)
	logger.Info().Msg("edgecore gateway starting...")

	// ── 3. Build router ───────────────────────────────────────────────
	router, err := gateway.NewRouter(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build router: %w", err)
	}

	// ── 4. Build server ───────────────────────────────────────────────
	server := gateway.NewServer(cfg.Server.Port, router, logger)

	return &App{
		Logger: logger,
		Config: cfg,
		Server: server,
	}, nil
}