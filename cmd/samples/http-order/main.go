package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/rs/zerolog"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Str("service", "http-order").Logger()
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"service":      "http-order-service",
			"method":       r.Method,
			"path":         r.URL.Path,
			"forwarded_by": r.Header.Get("X-Gateway"),
		})
	})

	addr := ":9002"
	logger.Info().Str("addr", addr).Msg("http-order-service listening")
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
