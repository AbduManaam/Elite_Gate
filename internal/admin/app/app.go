package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"elitegate/internal/admin"
	adminserver "elitegate/internal/admin/server"
	"elitegate/internal/config"
	"elitegate/internal/container"
	"elitegate/internal/observability"
	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

type App struct {
	Logger       zerolog.Logger
	Server       *adminserver.Server
	DB           *sql.DB
	ContainerMgr container.ContainerManager
	Cancel       context.CancelFunc
}

// StartApp sets up the admin app, database, logging, and HTTP server.Cleans up resources on failure.
// Returns an App with the logger, database, and server configured.
func StartApp(cfg *config.Config) (*App, error) {
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logCfg := cfg.Log
	logCfg.File = "logs/admin.log"
	logger := observability.NewServiceLogger(logCfg, "elitegate-admin")
	logger.Info().Msg("elitegate admin starting...")

	if cfg.Database.DSN == "" {
		return nil, fmt.Errorf("database connection DSN (POSTGRES_DSN) is required")
	}

	// Connect to postgres using injected database configs
	db, err := storage.NewPostgres(logger, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Create the Docker Container Manager
	containerMgr, err := container.NewDockerContainerManager(
		cfg.Server.AdminAPIURL,
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Auth.JWTSecret,
		"",
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create docker container manager: %w", err)
	}

	// Initialize router passing containerMgr
	router, err := admin.NewRouter(logger, db, cfg, containerMgr)
	if err != nil {
		_ = containerMgr.Close()
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
		_ = containerMgr.Close()
		_ = db.Close()
		return nil, fmt.Errorf("failed to create admin server: %w", err)
	}
	// Setup a cancellable context for pruner goroutines
	ctx, cancel := context.WithCancel(context.Background())

	// Start background pruners only after server/router are successfully created
	authRepo := storage.NewAdminAuthRepo(db)
	auditLogRepo := storage.NewAuditLogRepo(db, logger)
	admin.StartRefreshTokenPruner(ctx, authRepo, logger)
	admin.StartAuditLogPruner(ctx, auditLogRepo, logger)

	return &App{
		Logger:       logger,
		Server:       server,
		DB:           db,
		ContainerMgr: containerMgr,
		Cancel:       cancel,
	}, nil
}

// Close gracefully cancels background workers and closes DB and Docker SDK connections
func (a *App) Close() {
	a.Logger.Info().Msg("Shutting down admin application")
	if a.Cancel != nil {
		a.Cancel() // Cancels context, stopping StartRefreshTokenPruner & StartAuditLogPruner loops
	}
	if a.ContainerMgr != nil {
		if closer, ok := a.ContainerMgr.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}
	if a.DB != nil {
		_ = a.DB.Close()
	}
	a.Logger.Info().Msg("admin application cleanup complete")

}
