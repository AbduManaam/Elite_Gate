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
	mu       sync.RWMutex
	statuses map[string]*Status // key = upstream base URL

	client       *http.Client
	interval     time.Duration
	probeTimeout time.Duration
	healthPath   string
	logger       zerolog.Logger
}

// Creates a health checker. Call Start() to begin "periodic probes"(Repeatedly runs health checks at fixed intervals).
//
// interval: how often health checks run.
// healthPath: endpoint used for health checks.
// probeTimeout: maximum time allowed for each probe.
func New(interval time.Duration, healthPath string, probeTimeout time.Duration, logger zerolog.Logger) *Checker {
	return &Checker{
		statuses:     make(map[string]*Status),
		interval:     interval,
		probeTimeout: probeTimeout,
		healthPath:   healthPath,
		logger:       logger.With().Str("component", "health_checker").Logger(),
		client: &http.Client{
			Timeout: 0,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Adds an upstream URL to be health-checked.
// Safe to call anytime; duplicate URLs are ignored.
func (c *Checker) Register(baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.statuses[baseURL]; exists {
		return
	}
	c.statuses[baseURL] = &Status{Healthy: true}
	c.logger.Info().
		Str("upstream", baseURL).
		Msg("health: registered new upstream")
}

// Unregister removes an upstream from the watch list.
func (c *Checker) Unregister(baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.statuses[baseURL]; !exists {
		return
	}

	delete(c.statuses, baseURL)
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
		Str("health_path", c.healthPath).
		Msg("health: checker starting")
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
func (c *Checker) probe(ctx context.Context, baseURL string) (bool, string) {
	target := baseURL + c.healthPath

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
