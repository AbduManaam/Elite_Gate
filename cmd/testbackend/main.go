package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/rs/zerolog"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Str("service", "test-backend").Logger()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"service":      "test-backend",
			"method":       r.Method,
			"path":         r.URL.Path,
			"received":     true,
			"forwarded_by": r.Header.Get("X-Gateway"),
		})
	})

	logger.Info().Str("addr", ":9090").Msg("test backend listening")
	if err := http.ListenAndServe(":9090", mux); err != nil {
		logger.Fatal().Err(err).Msg("test backend failed")
	}
}
