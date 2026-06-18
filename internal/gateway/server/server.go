package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"elitegate/internal/config"
	"elitegate/internal/gateway/health"
	"elitegate/internal/gateway/runtime"

	"github.com/rs/zerolog"
)

type Server struct {
	http            *http.Server
	grpc            *GRPCGateway
	logger          zerolog.Logger
	shutdownTimeout time.Duration
}

// Sets up the HTTP and gRPC gateways.
// DevHostMap converts Docker service names to localhost,
// helping the gateway find services that are running on the local machine.

func NewServer(port string, handler http.Handler, logger zerolog.Logger, cfg config.ServerConfig, loader *runtime.Loader, interceptors *GRPCSecurityInterceptors, hc *health.Checker) (*Server, error) {
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

	grpcPort := cfg.GRPCGatewayPort
	if grpcPort == "" {
		grpcPort = ":50051"
	}
	grpcGateway := NewGRPCGateway(logger, loader, grpcPort, cfg.DevHostMap, interceptors, hc)

	return &Server{
		logger:          logger,
		shutdownTimeout: shutdownTimeout,
		grpc:            grpcGateway,
		http: &http.Server{
			Addr:         port,
			Handler:      handler,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
			IdleTimeout:  idleTimeout,
		},
	}, nil
}

// Run starts the server and blocks until a shutdown signal is received.
func (s *Server) Run() error {

	//  1. Start listening in background
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info().Str("addr", s.http.Addr).Msg("gateway listening")

		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("gateway listen failed: %w", err)
		}
	}()

	//  2. Block until OS signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start gRPC server in background
	go func() {
		if err := s.grpc.Start(ctx); err != nil {
			s.logger.Error().Err(err).Msg("gRPC gateway server failed")
		}
	}()

	var sig os.Signal
	select {
	case err := <-errCh:
		cancel()
		return err
	case sig = <-quit:
	}

	s.logger.Info().Str("signal", sig.String()).Msg("shutdown signal received")
	cancel() // Stop gRPC server

	//  3. Graceful shutdown (30s timeout)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer shutdownCancel()

	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	s.logger.Info().Msg("gateway stopped cleanly")
	return nil
}
