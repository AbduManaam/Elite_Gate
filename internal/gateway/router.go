package gateway

import (
	"net/http"

	"elitegate/internal/config"
	"elitegate/internal/gateway/middleware"
	"elitegate/internal/gateway/proxy"
	"elitegate/internal/ratelimit"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

func NewRouter(logger zerolog.Logger, rdb *redis.Client, cfg *config.Config) (http.Handler, error) {
	// ── 1. Reverse proxy ──────────────────────────────────────────────
	adminURL := cfg.Server.AdminAPIURL

	p, err := proxy.New(adminURL)
	if err != nil {
		return nil, err
	}

	// ── 2. Rate Limiter setup ─────────────────────────────────────────
	rpm := cfg.RateLimit.RequestsPerMinute
	memFallback := ratelimit.NewMemoryLimiter(rpm)
	limiter := ratelimit.NewRedisLimiter(rdb, rpm, memFallback)
	rlMiddleware := middleware.NewRateLimitMiddleware(limiter)

	// ── 3. Routes ─────────────────────────────────────────────────────
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ready"}`))
	})

	apiHandler := middleware.Chain(
		http.StripPrefix("/api", p),
		middleware.IPFilter,
		middleware.Auth(cfg.Auth.JWTSecret),
		rlMiddleware.Middleware,
	)
	mux.Handle("/api/", apiHandler)

	// ── 4. Middleware chain ───────────────────────────────────────────
	handler := middleware.Chain(
		mux,
		middleware.Recovery(logger),
		middleware.RequestLogger(logger),
	)

	return handler, nil
}