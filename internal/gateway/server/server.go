package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

type Server struct {
	http   *http.Server
	logger zerolog.Logger
}

func NewServer(port string, handler http.Handler, logger zerolog.Logger) *Server {
	return &Server{
		logger: logger,
		http: &http.Server{
			Addr:         port,
			Handler:      handler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
}

// Run starts the server and blocks until a shutdown signal is received.
func (s *Server) Run() error {

	// ── 1. Start listening in background ─────────────────────────────
	go func() {
		s.logger.Info().Str("addr", s.http.Addr).Msg("gateway listening")
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatal().Err(err).Msg("gateway failed")
		}
	}()

	// ── 2. Block until OS signal ──────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	s.logger.Info().Str("signal", sig.String()).Msg("shutdown signal received")

	// ── 3. Graceful shutdown (30s timeout) ────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	s.logger.Info().Msg("gateway stopped cleanly")
	return nil
}