package runtime

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"elitegate/internal/storage"
)

type Loader struct {
	repo     *storage.RouteRepo
	logger   zerolog.Logger
	interval time.Duration

	mu       sync.RWMutex
	snapshot Snapshot
}

func NewLoader(repo *storage.RouteRepo, logger zerolog.Logger, interval time.Duration) *Loader {
	return &Loader{
		repo:     repo,
		logger:   logger,
		interval: interval,
	}
}

func (l *Loader) Start(ctx context.Context) error {
	if err := l.reload(ctx); err != nil {
		return err
	}
	go l.loop(ctx)
	return nil
}

func (l *Loader) loop(ctx context.Context) {
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := l.reload(ctx); err != nil {
				l.logger.Error().Err(err).Msg("route reload failed")
			}
		}
	}
}

// Reloads routes from the database and updates the cache.
func (l *Loader) reload(ctx context.Context) error {
	routes, err := l.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.snapshot = Snapshot{Routes: routes}
	l.mu.Unlock()
	l.logger.Info().Int("routes", len(routes)).Msg("gateway routes reloaded")
	return nil
}

func (l *Loader) Current() Snapshot {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.snapshot
}

// Public wrapper for triggering a route cache reload.
func (l *Loader) Reload(ctx context.Context) error {
	return l.reload(ctx)
}
