package runtime

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"elitegate/internal/gateway/health"
	"elitegate/internal/gateway/loadbalancer"
	"elitegate/internal/storage"
)

type Loader struct {
	repo       *storage.RouteRepo
	targetRepo *storage.UpstreamTargetRepo
	upstreams  *storage.UpstreamRepo
	logger     zerolog.Logger
	interval   time.Duration
	health     *health.Checker

	mu           sync.RWMutex
	snapshot     Snapshot
	strategiesMu sync.Mutex
	strategies   map[string]loadbalancer.Strategy
}

func NewLoader(repo *storage.RouteRepo, targetRepo *storage.UpstreamTargetRepo, upstreams *storage.UpstreamRepo, logger zerolog.Logger, interval time.Duration) *Loader {
	return &Loader{
		repo:       repo,
		targetRepo: targetRepo,
		upstreams:  upstreams,
		logger:     logger,
		interval:   interval,
		strategies: make(map[string]loadbalancer.Strategy),
	}
}

func (l *Loader) SetHealthChecker(hc *health.Checker) {
	l.health = hc
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

// Reloads routes and upstream target pools from the database and
// atomically swaps the in-memory snapshot.
func (l *Loader) reload(ctx context.Context) error {
	routes, err := l.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}

	pools, err := l.buildUpstreamPools(ctx)
	if err != nil {

		l.logger.Error().Err(err).Msg("failed to build upstream LB pools; keeping previous pool state")
		l.mu.Lock()
		prevPools := l.snapshot.UpstreamPools
		l.snapshot = Snapshot{Routes: routes, UpstreamPools: prevPools}
		l.mu.Unlock()
		l.logger.Info().Int("routes", len(routes)).Msg("gateway routes reloaded (pools unchanged)")
		return nil
	}

	l.mu.Lock()
	l.snapshot = Snapshot{Routes: routes, UpstreamPools: pools}
	l.mu.Unlock()

	l.syncHealthTargets(pools)

	l.logger.Info().
		Int("routes", len(routes)).
		Int("upstream_pools", len(pools)).
		Msg("gateway routes reloaded")
	return nil
}

/*
1.Takes all backend servers from pools
2.Extracts their URLs
3.Sends them to health system:
The function tells the health-check system:
Here is the latest list of backend servers you should monitor. Forget old ones, use this updated list*/

func (l *Loader) syncHealthTargets(pools map[string]UpstreamPool) {
	if l.health == nil {
		return
	}
	// Build map[targetURL → TargetHealthConfig] from all pools.
	// Every target in a pool shares the upstream's health path.
	targets := make(map[string]health.TargetHealthConfig, len(pools)*2)
	for _, pool := range pools {
		for _, t := range pool.Targets {
			targets[t.URL] = health.TargetHealthConfig{
				HealthPath: pool.HealthPath,
				ProjectID:  pool.ProjectID,
			}
		}
	}
	l.health.SyncTargets(targets)
}

func (l *Loader) buildUpstreamPools(ctx context.Context) (map[string]UpstreamPool, error) {
	upstreams, err := l.upstreams.ListAllEnabledGlobal(ctx)
	if err != nil {
		return nil, err
	}

	targetsByUpstream, err := l.targetRepo.ListAllEnabledGlobal(ctx)
	if err != nil {
		return nil, err
	}

	pools := make(map[string]UpstreamPool, len(upstreams))

	for _, u := range upstreams {
		strategy := l.strategyFor(u.ID, u.LBStrategy)

		rawTargets := targetsByUpstream[u.ID]
		targets := make([]loadbalancer.Target, 0, len(rawTargets))
		for _, t := range rawTargets {
			if !t.Enabled {
				continue
			}
			targets = append(targets, loadbalancer.Target{
				ID:     t.ID,
				URL:    t.TargetURL,
				Weight: t.Weight,
			})
		}

		if len(targets) == 0 && u.TargetURL != "" {
			targets = append(targets, loadbalancer.Target{
				ID:     u.ID,
				URL:    u.TargetURL,
				Weight: 1,
			})
		}

		pools[u.ID] = UpstreamPool{
			ProjectID:  u.ProjectID,
			Targets:    targets,
			Strategy:   strategy,
			HealthPath: u.HealthPath,
		}
	}

	return pools, nil
}

func (l *Loader) strategyFor(upstreamID, lbStrategy string) loadbalancer.Strategy {
	l.strategiesMu.Lock()
	defer l.strategiesMu.Unlock()

	existing, ok := l.strategies[upstreamID]
	if ok && existing.Name() == normalizeLBStrategy(lbStrategy) {
		return existing
	}

	fresh := loadbalancer.NewStrategy(lbStrategy)
	l.strategies[upstreamID] = fresh

	if ok {
		l.logger.Info().
			Str("upstream_id", upstreamID).
			Str("old_strategy", existing.Name()).
			Str("new_strategy", fresh.Name()).
			Msg("upstream load balancing strategy changed; in-flight counters reset")
	}

	return fresh
}

func normalizeLBStrategy(s string) string {
	if s == "least_conn" {
		return "least_conn"
	}
	return "round_robin"
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
