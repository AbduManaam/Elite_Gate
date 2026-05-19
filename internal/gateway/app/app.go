package app

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"edgecore/internal/config"
	gatewayRouter "edgecore/internal/gateway"
	gateway "edgecore/internal/gateway/server"
	"edgecore/internal/observability"
	"edgecore/internal/storage"
)

type App struct {
	Logger zerolog.Logger
	Config *config.Config
	Server *gateway.Server
	DB     *sql.DB
}

func StartApp(cfg *config.Config) (*App, error) {

	// ── 1. Create logs directory ──────────────────────────────────────
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// ── 2. Build logger ───────────────────────────────────────────────
	logger := observability.NewLogger(cfg.Log)
	logger.Info().Msg("edgecore gateway starting...")

	// ── 3. Connect to database ───────────────────────────────────────
	db, err := storage.NewPostgres(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// ── 4. Build router ───────────────────────────────────────────────
	router, err := gatewayRouter.NewRouter(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build router: %w", err)
	}

	// ── 5. Build server ───────────────────────────────────────────────
	server := gateway.NewServer(cfg.Server.Port, router, logger)

	return &App{
		Logger: logger,
		Config: cfg,
		Server: server,
		DB:     db,
	}, nil
}
