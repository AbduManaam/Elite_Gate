package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"elitegate/helper"
	"elitegate/internal/auth"
	"elitegate/internal/domain"
	"elitegate/internal/gateway/health"
	"elitegate/internal/gateway/loadbalancer"
	"elitegate/internal/model"
)

type JWTConfigApplier interface {
	Apply(
		ctx context.Context,
		cfg *model.ProjectJWTConfigSync,
	) error
}

type Loader struct {
	controlClient *ControlPlaneClient
	redis         *redis.Client
	keyStore      *auth.RedisKeyStore
	logger        zerolog.Logger
	interval      time.Duration
	health        *health.Checker

	jwtConfigApplier JWTConfigApplier
	reloadMu         sync.Mutex

	mu           sync.RWMutex
	snapshot     Snapshot
	strategiesMu sync.Mutex
	strategies   map[string]loadbalancer.Strategy
}

func NewLoader(controlClient *ControlPlaneClient, rdb *redis.Client, logger zerolog.Logger, interval time.Duration) *Loader {
	return &Loader{
		controlClient: controlClient,
		redis:         rdb,
		logger:        logger,
		interval:      interval,
		strategies:    make(map[string]loadbalancer.Strategy),
	}
}

func (l *Loader) SetKeyStore(ks *auth.RedisKeyStore) {
	l.keyStore = ks
}

func (l *Loader) SetJWTConfigApplier(
	applier JWTConfigApplier,
) {
	l.jwtConfigApplier = applier
}

func (l *Loader) SetHealthChecker(hc *health.Checker) {
	l.health = hc
}

func (l *Loader) SetSnapshotForTest(snap *Snapshot) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if snap != nil {
		l.snapshot = *snap
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

// Reloads configuration snapshot from the control plane and
// atomically swaps the in-memory snapshot, warming up the api key cache in Redis.
func (l *Loader) reload(ctx context.Context) error {
	l.reloadMu.Lock()
	defer l.reloadMu.Unlock()

	snap, err := l.controlClient.FetchSnapshot(ctx)
	if err != nil {
		l.logger.Error().Err(err).Msg("failed to fetch configuration snapshot from control plane")
		return err // keep serving the last good snapshot
	}

	if l.jwtConfigApplier != nil {
		if err := l.jwtConfigApplier.Apply(
			ctx,
			snap.JWTAuth,
		); err != nil {

			l.logger.Error().
				Err(err).
				Msg(
					"failed to apply project JWT configuration",
				)

			// Important:
			// do not activate the new route snapshot.
			return fmt.Errorf(
				"apply project JWT configuration: %w",
				err,
			)
		}
	}

	pools := l.buildPoolsFromSnapshot(snap)
	domainMap := l.buildDomainMap(snap)

	l.mu.Lock()
	l.snapshot = Snapshot{
		Routes:        snap.Routes,
		UpstreamPools: pools,
		CustomDomains: snap.CustomDomains,
		DomainMap:     domainMap,
	}
	l.mu.Unlock()

	l.syncHealthTargets(pools)
	l.WarmAPIKeyCache(ctx, snap.APIKeys)

	l.logger.Info().
		Int("routes", len(snap.Routes)).
		Int("upstream_pools", len(pools)).
		Int("api_keys_warmed", len(snap.APIKeys)).
		Int("custom_domains", len(snap.CustomDomains)).
		Msg("gateway config reloaded from control plane")
	return nil
}

// WarmAPIKeyCache pushes every active key from the latest snapshot into
// local memory and Redis under this tenant's own prefix.
func (l *Loader) WarmAPIKeyCache(ctx context.Context, keys []TenantAPIKeyDTO) {
	dtos := make([]auth.KeySnapshotDTO, len(keys))
	for i, k := range keys {
		dtos[i] = auth.KeySnapshotDTO{
			KeyHash: k.KeyHash,
			Roles:   k.Roles,
			Scopes:  k.Scopes,
		}
	}
	projectID := ""
	if l.controlClient != nil {
		projectID = l.controlClient.ProjectID()
	}

	if l.keyStore != nil {
		l.keyStore.UpdateLocalKeys(projectID, dtos)
	}

	if l.redis == nil {
		return
	}
	for _, k := range keys {
		rec := auth.APIKeyRecord{
			ClientID: projectID,
			Roles:    k.Roles,
			Scopes:   k.Scopes,
		}
		data, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		cacheKey := helper.PrefixedKey("apikey:" + k.KeyHash)
		if err := l.redis.Set(ctx, cacheKey, data, 10*time.Minute).Err(); err != nil {
			l.logger.Warn().Err(err).Msg("failed to warm api key cache entry")
		}
	}
}

func (l *Loader) syncHealthTargets(pools map[string]UpstreamPool) {
	if l.health == nil {
		return
	}
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

func (l *Loader) buildPoolsFromSnapshot(snap *TenantSnapshot) map[string]UpstreamPool {
	pools := make(map[string]UpstreamPool, len(snap.Upstreams))

	for _, u := range snap.Upstreams {
		strategy := l.strategyFor(u.ID, u.LBStrategy)

		rawTargets := snap.Targets[u.ID]
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

	return pools
}

func (l *Loader) buildDomainMap(snap *TenantSnapshot) map[string]DomainContext {
	domainMap := make(map[string]DomainContext, len(snap.CustomDomains))
	for _, cd := range snap.CustomDomains {
		norm := NormalizeHost(cd.Hostname)
		if norm == "" {
			continue
		}
		if cd.Status != domain.CustomDomainStatusActive ||
			cd.RoutingStatus != domain.CustomDomainRoutingStatusReady {
			continue
		}
		domainMap[norm] = DomainContext{
			Hostname:      cd.Hostname,
			Status:        cd.Status,
			RoutingStatus: cd.RoutingStatus,
		}
	}
	return domainMap
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

func (l *Loader) Reload(ctx context.Context) error {
	return l.reload(ctx)
}
