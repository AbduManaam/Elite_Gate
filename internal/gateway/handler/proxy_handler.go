package handler

import (
	"net/http"
	"sync"

	"elitegate/internal/gateway/health"
	"elitegate/internal/gateway/loadbalancer"
	"elitegate/internal/gateway/proxy"
	"elitegate/internal/gateway/runtime"
	"elitegate/internal/model"
	"elitegate/internal/shared"

	"github.com/rs/zerolog"
)

type DynamicProxy struct {
	loader  *runtime.Loader
	hostMap map[string]string
	mu      sync.Mutex
	proxies map[string]*proxy.ReverseProxy
	health  *health.Checker
	logger  zerolog.Logger
}

func NewDynamicProxy(loader *runtime.Loader, hostMap map[string]string, hc *health.Checker, logger zerolog.Logger) *DynamicProxy {
	return &DynamicProxy{
		loader:  loader,
		hostMap: hostMap,
		proxies: make(map[string]*proxy.ReverseProxy),
		health:  hc,
		logger:  logger.With().Str("component", "dynamic_proxy").Logger(),
	}
}

func (d *DynamicProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt, ok := r.Context().Value(shared.ContextKeyRoute).(*model.Route)
	if !ok || rt == nil {
		http.Error(w, `{"error":"route not found"}`, http.StatusNotFound)
		return
	}

	target, strategy, err := d.resolveTarget(rt)
	if err != nil {
		// No healthy backend in the pool — this is the one LB-related
		// failure worth a log line, since it means the route is
		// effectively down and an operator needs to know.
		d.logger.Warn().
			Err(err).
			Str("route_id", rt.ID).
			Str("route_path", rt.Path).
			Str("upstream_id", derefStr(rt.UpstreamID)).
			Msg("no healthy backend available for route")

		http.Error(w,
			`{"error":"upstream unavailable","detail":"no healthy backend in pool"}`,
			http.StatusServiceUnavailable,
		)
		return
	}

	if strategy != nil {
		defer strategy.Release(target)
	}

	p, err := d.getProxy(target.URL)
	if err != nil {
		d.logger.Error().
			Err(err).
			Str("target_url", target.URL).
			Str("route_id", rt.ID).
			Msg("failed to build reverse proxy for target")
		http.Error(w, `{"error":"bad upstream"}`, http.StatusBadGateway)
		return
	}
	p.ServeHTTP(w, r)
}

func (d *DynamicProxy) resolveTarget(rt *model.Route) (loadbalancer.Target, loadbalancer.Strategy, error) {
	snap := d.loader.Current()

	if rt.UpstreamID != nil {
		if pool, ok := snap.UpstreamPools[*rt.UpstreamID]; ok && len(pool.Targets) > 0 {
			healthy := d.filterHealthy(pool.Targets)
			if len(healthy) == 0 {
				return loadbalancer.Target{}, nil, loadbalancer.ErrNoHealthyTargets
			}

			t, err := pool.Strategy.Pick(healthy)
			if err != nil {
				return loadbalancer.Target{}, nil, err
			}
			d.logger.Info().
				Str("picked", t.URL).
				Msg("LB selected target")
			return t, pool.Strategy, nil
		}
	}

	// Legacy fallback: route's joined upstream_url (single target, no LB).
	if rt.UpstreamURL == "" {
		return loadbalancer.Target{}, nil, loadbalancer.ErrNoHealthyTargets
	}
	if d.health != nil && !d.health.IsHealthy(rt.UpstreamURL) {
		return loadbalancer.Target{}, nil, loadbalancer.ErrNoHealthyTargets
	}
	return loadbalancer.Target{ID: rt.UpstreamURL, URL: rt.UpstreamURL}, nil, nil
}

func (d *DynamicProxy) filterHealthy(targets []loadbalancer.Target) []loadbalancer.Target {
	if d.health == nil {
		return targets
	}
	out := make([]loadbalancer.Target, 0, len(targets))
	for _, t := range targets {
		if d.health.IsHealthy(t.URL) {
			out = append(out, t)
		}
	}
	return out
}

// This function caches reverse proxies: it returns an existing proxy for a target if one already exists,
// otherwise it creates, stores, and returns a new one.
func (d *DynamicProxy) getProxy(target string) (*proxy.ReverseProxy, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if p, ok := d.proxies[target]; ok {
		return p, nil
	}
	p, err := proxy.New(target, d.hostMap)
	if err != nil {
		return nil, err
	}
	d.proxies[target] = p
	return p, nil
}

// Safely get the value of a *string pointer without causing a panic if the pointer is nil
// If the pointer is nil → returns an empty string ""
// If the pointer contains a value → returns that value
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
