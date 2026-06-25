package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

type Status struct {
	Healthy          bool
	LastCheck        time.Time
	LastErr          string
	ConsecutiveFails int // Tracks consecutive health check failures (failures happening one after another)
}

// Runs concurrent health checks for upstream services and store their status.
type Checker struct {
	mu          sync.RWMutex
	statuses    map[string]*Status  // key = upstream base URL
	healthPaths map[string]string   // key = upstream base URL, value = health path (e.g. "/health")

	client       *http.Client
	interval     time.Duration
	probeTimeout time.Duration
	logger       zerolog.Logger
}

// Creates a health checker. Call Start() to begin "periodic probes"(Repeatedly runs health checks at fixed intervals).
//
// interval: how often health checks run.
// probeTimeout: maximum time allowed for each probe.
// Each upstream registers its own health path via Register().
func New(interval time.Duration, probeTimeout time.Duration, logger zerolog.Logger) *Checker {
	return &Checker{
		statuses:     make(map[string]*Status),
		healthPaths:  make(map[string]string),
		interval:     interval,
		probeTimeout: probeTimeout,
		logger:       logger.With().Str("component", "health_checker").Logger(),
		client: &http.Client{
			Timeout: 0,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Register adds an upstream URL to be health-checked with its specific health path.
// If healthPath is empty, "/health" is used as a safe default.
// Safe to call anytime; if the URL already exists the health path is updated.
func (c *Checker) Register(baseURL, healthPath string) {
	if healthPath == "" {
		healthPath = "/health"
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.statuses[baseURL]; !exists {
		c.statuses[baseURL] = &Status{Healthy: true}
		c.logger.Info().
			Str("upstream", baseURL).
			Str("health_path", healthPath).
			Msg("health: registered new upstream")
	}
	c.healthPaths[baseURL] = healthPath
}

// RegisterAll registers multiple upstreams. The map key is the base URL,
// the value is the health path for that upstream.
func (c *Checker) RegisterAll(targets map[string]string) {
	for url, path := range targets {
		c.Register(url, path)
	}
}

// SyncTargets reconciles the health checker's watch list with the desired set.
// desired is a map of baseURL → healthPath.
// URLs no longer in the desired set are unregistered.
func (c *Checker) SyncTargets(desired map[string]string) {
	want := make(map[string]struct{}, len(desired))
	for u := range desired {
		want[u] = struct{}{}
	}

	c.mu.RLock()
	var toRemove []string
	for existing := range c.statuses {
		if _, keep := want[existing]; !keep {
			toRemove = append(toRemove, existing)
		}
	}
	c.mu.RUnlock()

	for url, path := range desired {
		c.Register(url, path)
	}
	for _, u := range toRemove {
		c.Unregister(u)
	}
}

// Unregister removes an upstream from the watch list.
func (c *Checker) Unregister(baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.statuses[baseURL]; !exists {
		return
	}

	delete(c.statuses, baseURL)
	delete(c.healthPaths, baseURL)
	c.logger.Info().
		Str("upstream", baseURL).
		Msg("health: unregistered upstream")
}

// Returns whether an upstream is healthy.
// Unchecked upstreams default to healthy.
func (c *Checker) IsHealthy(baseURL string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	s, ok := c.statuses[baseURL]
	if !ok {
		return true
	}
	return s.Healthy
}

// Statuses returns a copy of all known upstream statuses.
// Useful for exposing a /health/upstreams admin endpoint.
func (c *Checker) Statuses() map[string]Status {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make(map[string]Status, len(c.statuses))
	for k, v := range c.statuses {
		out[k] = *v
	}
	return out
}

// Start begins the background probe loop. It stops when ctx is cancelled.
// Call this once during application startup.
func (c *Checker) Start(ctx context.Context) {
	c.logger.Info().
		Dur("interval", c.interval).
		Dur("probe_timeout", c.probeTimeout).
		Msg("health: checker starting (per-upstream health paths)")
	go c.loop(ctx)
}

// Runs health checks for all upstream services at regular intervals.
func (c *Checker) loop(ctx context.Context) {
	c.probeAll(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info().Msg("health: checker stopped — context cancelled")
			return
		case <-ticker.C:
			c.probeAll(ctx)
		}
	}
}

// Checks all registered upstreams concurrently and updates their health status.
func (c *Checker) probeAll(ctx context.Context) {
	c.mu.RLock()
	urls := make([]string, 0, len(c.statuses))
	for u := range c.statuses {
		urls = append(urls, u)
	}
	c.mu.RUnlock()

	if len(urls) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			probeCtx, cancel := context.WithTimeout(ctx, c.probeTimeout)
			defer cancel()

			healthy, errMsg := c.probe(probeCtx, url)
			c.setStatus(url, healthy, errMsg)
		}(u)
	}
	wg.Wait()
}

// Performs a single health check and returns its status.
// Uses the per-upstream health path registered via Register().
func (c *Checker) probe(ctx context.Context, baseURL string) (bool, string) {
	c.mu.RLock()
	healthPath, ok := c.healthPaths[baseURL]
	c.mu.RUnlock()
	if !ok || healthPath == "" {
		healthPath = "/health"
	}

	target := baseURL + healthPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, fmt.Sprintf("build request: %s", err.Error())
	}
	req.Header.Set("User-Agent", "elitegate-health/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	healthy := resp.StatusCode >= 200 && resp.StatusCode < 300
	if healthy {
		return true, ""
	}

	return false, fmt.Sprintf("HTTP %s", resp.Status)
}

func (c *Checker) setStatus(baseURL string, healthy bool, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	prev, ok := c.statuses[baseURL]
	if !ok {
		return
	}

	stateChanged := prev.Healthy != healthy

	if healthy {
		prev.ConsecutiveFails = 0
	} else {
		prev.ConsecutiveFails++
	}

	if stateChanged {
		if healthy {
			c.logger.Info().
				Str("upstream", baseURL).
				Msg("health: upstream RECOVERED ✅")
		} else {
			c.logger.Error().
				Str("upstream", baseURL).
				Str("reason", errMsg).
				Int("consecutive_fails", prev.ConsecutiveFails).
				Msg("health: upstream went DOWN ❌")
		}
	}

	prev.Healthy = healthy
	prev.LastCheck = time.Now()
	if healthy {
		prev.LastErr = ""
	} else {
		prev.LastErr = errMsg
	}
}
