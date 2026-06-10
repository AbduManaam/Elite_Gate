package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"elitegate/internal/config"

	"github.com/rs/zerolog"
)

type Server struct {
	http            *http.Server
	logger          zerolog.Logger
	shutdownTimeout time.Duration
}

/*
NewServer():  function validates the server timeout settings, creates an HTTP server with those settings, and returns the ready-to-use server instance.

1.Reads timeout values from the config (ReadTimeout, WriteTimeout, IdleTimeout, ShutdownTimeout).
2.Converts those string values (such as "5s" or "30s") into Go time.Duration values using time.ParseDuration().
3.If any timeout value is invalid, it immediately returns an error explaining which timeout failed to parse.
4.Creates a new Server struct.
5.Stores:  Stores the logger and shutdown timeout, then creates an http.Server with the port, handler, and timeout settings.
6.Returns the configured server.
*/
func NewServer(port string, handler http.Handler, logger zerolog.Logger, cfg config.ServerConfig) (*Server, error) {
	readTimeout, err := time.ParseDuration(cfg.ReadTimeout)
	if err != nil {
		return nil, fmt.Errorf("parse read timeout: %w", err)
	}
	writeTimeout, err := time.ParseDuration(cfg.WriteTimeout)
	if err != nil {
		return nil, fmt.Errorf("parse write timeout: %w", err)
	}
	idleTimeout, err := time.ParseDuration(cfg.IdleTimeout)
	if err != nil {
		return nil, fmt.Errorf("parse idle timeout: %w", err)
	}
	shutdownTimeout, err := time.ParseDuration(cfg.ShutdownTimeout)
	if err != nil {
		return nil, fmt.Errorf("parse shutdown timeout: %w", err)
	}

	return &Server{
		logger:          logger,
		shutdownTimeout: shutdownTimeout,
		http: &http.Server{
			Addr:         port,
			Handler:      handler,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
			IdleTimeout:  idleTimeout,
		},
	}, nil
}

func (s *Server) Run() error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info().Str("addr", s.http.Addr).Msg("admin API listening")
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("admin API listen failed: %w", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	var sig os.Signal
	select {
	case err := <-errCh:
		return err
	case sig = <-quit:
	}

	s.logger.Info().Str("signal", sig.String()).Msg("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	s.logger.Info().Msg("admin API stopped cleanly")
	return nil
}
