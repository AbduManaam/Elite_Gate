package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/rs/zerolog"
)

func main() {

	logger := zerolog.New(os.Stdout).With().Timestamp().Str("service", "http-user").Logger()
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-type", "application/json")

		if err := json.NewEncoder(w).Encode(map[string]any{
			"service":      "http-user-service",
			"method":       r.Method,
			"path":         r.URL.Path,
			"forwarded by": r.Header.Get("X-Gateeway"),
		}); err != nil {
			logger.Error().Err(err).Msg("json encode failed")
		}
	})
	addr := ":9001"
	logger.Info().Str("addr", addr).Msg("http-user-service listening")
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
