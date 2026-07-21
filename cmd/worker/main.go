package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"elitegate/internal/config"
	"elitegate/internal/container"
	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

func main() {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		With().Timestamp().Str("component", "worker").Logger()

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}
	if cfg.Database.DSN == "" {
		logger.Fatal().Msg("database connection DSN (POSTGRES_DSN) is required")
	}

	db, err := storage.NewPostgres(logger, cfg.Database)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	containerMgr, err := container.NewDockerContainerManager(
		cfg.Server.AdminAPIURL,
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Auth.JWTSecret,
		cfg.Server.GatewayImageName,
		cfg.Server.GatewayHostPublic,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init container manager")
	}
	defer containerMgr.Close()

	gatewayRepo := storage.NewGatewayRepo(db)

	staleAfter, err := time.ParseDuration(cfg.Server.DrainStaleAfter)
	if err != nil {
		logger.Fatal().Err(err).Msg("invalid server.drain_stale_after")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	logger.Info().Dur("stale_after", staleAfter).Msg("worker started: reconciling stale draining gateways")

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("worker shutting down")
			return
		case <-ticker.C:
			reconcileStaleDraining(ctx, gatewayRepo, containerMgr, staleAfter, logger)
		}
	}
}

// reconcileStaleDraining finishes decommissioning any gateway that has
// been stuck in "draining" longer than staleAfter — the safety net for
// requests that started a drain but never got to finish it (process
// restart, client disconnect, network partition).
func reconcileStaleDraining(
	ctx context.Context,
	gatewayRepo *storage.GatewayRepo,
	containerMgr container.ContainerManager,
	staleAfter time.Duration,
	logger zerolog.Logger,
) {
	stale, err := gatewayRepo.ListStaleDraining(ctx, staleAfter)
	if err != nil {
		logger.Error().Err(err).Msg("failed to list stale draining gateways")
		return
	}
	if len(stale) == 0 {
		return
	}

	logger.Info().Int("count", len(stale)).Msg("reconciling stale draining gateways")

	for _, g := range stale {
		log := logger.With().Str("external_id", g.ExternalID).
			Time("drain_started_at", g.DrainStartedAt).Logger()

		log.Warn().Msg("gateway stuck draining past stale threshold; finishing decommission")

		if err := containerMgr.Decommission(ctx, g.ExternalID); err != nil {
			log.Error().Err(err).Msg("reconciler: failed to stop container runtime")
			continue // retry next tick
		}
		if err := gatewayRepo.Decommission(ctx, g.ExternalID); err != nil {
			log.Error().Err(err).Msg("reconciler: failed to finalize DB row")
			continue // retry next tick
		}

		log.Info().Msg("reconciler: gateway decommissioned successfully")
	}
}
