package admin

import (
	"context"
	"time"

	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

func StartRefreshTokenPruner(
	ctx context.Context,
	repo *storage.AdminAuthRepo,
	logger zerolog.Logger,
) {

	// Run every 6 hours
	ticker := time.NewTicker(6 * time.Hour)

	// Run cleanup in background
	go func() {

		// Stop ticker when goroutine exits
		defer ticker.Stop()

		for {

			select {

			// Stop when application shuts down
			case <-ctx.Done():
				return

			// Every 6 hours
			case <-ticker.C:

				// Delete expired refresh tokens
				if err := repo.PruneExpiredTokens(ctx); err != nil {

					// Log cleanup failure
					logger.Error().
						Err(err).
						Msg("failed to prune refresh tokens")
				}
			}
		}
	}()
}

//function runs a background task that automatically cleans expired refresh tokens from the database every 6 hours.

const (
	pruneInterval = 24 * time.Hour
	pruneAge      = 180 * 24 * time.Hour // 6 months
)

// StartAuditLogPruner launches a background goroutine that deletes audit log
// entries older than 6 months. It runs immediately on start and then every 24 h.
// The goroutine stops cleanly when ctx is cancelled.
func StartAuditLogPruner(
	ctx context.Context,
	repo *storage.AuditLogRepo,
	logger zerolog.Logger,
) {
	log := logger.With().Str("worker", "audit_log_pruner").Logger()

	runPrune := func() {
		// Recover from any unexpected panics so the ticker loop stays alive.
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Msg("recovered from panic during audit log prune")
			}
		}()

		log.Info().
			Dur("prune_age", pruneAge).
			Msg("initiating historical audit log background cleanup")

		deleted, err := repo.PruneAuditLogs(ctx, pruneAge)
		if err != nil {
			log.Error().
				Err(err).
				Dur("prune_age", pruneAge).
				Msg("failed to prune old audit logs")
			return
		}

		log.Info().
			Int64("rows_deleted", deleted).
			Dur("prune_age", pruneAge).
			Msg("historical audit log background cleanup completed successfully")
	}

	ticker := time.NewTicker(pruneInterval)

	go func() {
		defer ticker.Stop()

		// Run immediately so we don't wait up to 24 h on the first boot.
		log.Info().Msg("running initial audit log prune on startup")
		runPrune()

		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("audit log pruner shutting down")
				return
			case <-ticker.C:
				runPrune()
			}
		}
	}()

	log.Info().
		Dur("interval", pruneInterval).
		Dur("prune_age", pruneAge).
		Msg("audit log pruner started")
}
