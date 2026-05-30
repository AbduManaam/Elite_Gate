package gateway

import (
	"net/http"

	"elitegate/internal/config"
	"elitegate/internal/gateway/handler"
	"elitegate/internal/gateway/middleware"
	"elitegate/internal/gateway/runtime"
	"elitegate/internal/ratelimit"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

func NewRouter(
	logger zerolog.Logger,
	rdb *redis.Client,
	cfg *config.Config,
	loader *runtime.Loader,
) (http.Handler, error) {
	dynamic := handler.NewDynamicProxy(loader, cfg.Server.DevHostMap)

	rpm := cfg.RateLimit.RequestsPerMinute
	memFallback := ratelimit.NewMemoryLimiter(rpm)
	limiter := ratelimit.NewRedisLimiter(rdb, rpm, memFallback)
	rlMiddleware := middleware.NewRateLimitMiddleware(limiter)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"gateway"}`))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	apiHandler := middleware.Chain(
		dynamic,
		middleware.IPFilter,
		middleware.Auth(cfg.Auth.JWTSecret),
		rlMiddleware.Middleware,
	)
	mux.Handle("/api/", apiHandler)

	return middleware.Chain(
		mux,
		middleware.Recovery(logger),
		middleware.RequestLogger(logger),
	), nil
}
