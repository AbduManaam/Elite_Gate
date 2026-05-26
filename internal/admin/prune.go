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