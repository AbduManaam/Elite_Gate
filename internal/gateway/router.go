package gateway

import (
	"net/http"
	"os"

	"elitegate/internal/gateway/middleware"
	"elitegate/internal/gateway/proxy"

	"github.com/rs/zerolog"
)

func NewRouter(logger zerolog.Logger) (http.Handler, error) {

	// ── 1. Reverse proxy ──────────────────────────────────────────────
	adminURL := os.Getenv("ADMIN_API_URL")
	if adminURL == "" {
		adminURL = "http://admin:9090" // Default for Docker networking
	}

	p, err := proxy.New(adminURL)
	if err != nil {
		return nil, err
	}

	// ── 2. Routes ─────────────────────────────────────────────────────
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
		middleware.Auth,
		middleware.RateLimit,
	)
	mux.Handle("/api/", apiHandler)

	// ── 3. Middleware chain ───────────────────────────────────────────
	handler := middleware.Chain(
		mux,
		middleware.Recovery,
		middleware.RequestLogger(logger),
	)

	return handler, nil
}
