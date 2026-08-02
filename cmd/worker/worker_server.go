package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

type WorkerMetricsServer struct {
	server *http.Server
	logger zerolog.Logger
}

func StartWorkerMetricsServer(reg *prometheus.Registry, logger zerolog.Logger) *WorkerMetricsServer {
	addr := os.Getenv("WORKER_METRICS_ADDR")
	if addr == "" {
		addr = ":9091"
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service": "worker",
			"status":  "ok",
			"version": "1.0.0",
		})
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	ws := &WorkerMetricsServer{
		server: srv,
		logger: logger,
	}

	go func() {
		logger.Info().Str("addr", addr).Msg("starting internal worker metrics & health HTTP server")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("internal worker metrics HTTP server failed")
		}
	}()

	return ws
}

func (ws *WorkerMetricsServer) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ws.logger.Info().Msg("shutting down internal worker metrics HTTP server")
	return ws.server.Shutdown(shutdownCtx)
}
