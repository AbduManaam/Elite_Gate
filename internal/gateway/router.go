package gateway

import (
	"net/http"

	"edgecore/internal/gateway/middleware"
	"edgecore/internal/gateway/proxy"

	"github.com/rs/zerolog"
)

func NewRouter(logger zerolog.Logger) (http.Handler, error) {

	// ── 1. Reverse proxy ──────────────────────────────────────────────
	p, err := proxy.New("http://localhost:9090")
	if err != nil {
		return nil, err
	}

	// ── 2. Routes ─────────────────────────────────────────────────────
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	mux.Handle("/api/", http.StripPrefix("/api", p))

	// ── 3. Middleware chain ───────────────────────────────────────────
	handler := middleware.Chain(
		mux,
		middleware.Recovery,
		middleware.RequestLogger(logger),
		middleware.IPFilter,
		middleware.Auth,
		middleware.RateLimit,
	)

	return handler, nil
}
