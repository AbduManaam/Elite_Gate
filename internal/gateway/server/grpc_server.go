package gateway

import (
	"context"
	"net"
	"sync"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"elitegate/internal/gateway/proxy"
	"elitegate/internal/gateway/runtime"
)

type GRPCGateway struct {
	logger zerolog.Logger
	loader *runtime.Loader
	port   string

	// FIX: cache the backends map and rebuild only when routes change.
	mu       sync.RWMutex
	backends map[string]string
}

func NewGRPCGateway(logger zerolog.Logger, loader *runtime.Loader, port string) *GRPCGateway {
	g := &GRPCGateway{logger: logger, loader: loader, port: port}
	g.backends = g.buildGRPCBackends()
	return g
}

// RefreshBackends rebuilds the cached backends map from the current route table.
// Call this whenever the Loader reloads routes.
func (g *GRPCGateway) RefreshBackends() {
	m := g.buildGRPCBackends()
	g.mu.Lock()
	g.backends = m
	g.mu.Unlock()
}

// Start listens and serves until ctx is cancelled, then performs a graceful stop.
func (g *GRPCGateway) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", g.port)
	if err != nil {
		return err
	}

	s := grpc.NewServer(
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			// FIX: grpc.MethodFromServerStream is not public API.
			fullMethod, ok := grpc.Method(stream.Context())
			if !ok {
				return status.Error(codes.Internal, "method not found")
			}

			// FIX: read from cache instead of rebuilding on every RPC.
			g.mu.RLock()
			backends := g.backends
			g.mu.RUnlock()

			addr, ok := proxy.ResolveGRPCBackend(fullMethod, backends)
			if !ok {
				return status.Error(codes.NotFound, "no grpc route")
			}
			return proxy.TransparentHandler(addr)(srv, stream)
		}),
	)

	// FIX: graceful shutdown when ctx is cancelled.
	go func() {
		<-ctx.Done()
		s.GracefulStop()
	}()

	g.logger.Info().Str("port", g.port).Msg("gRPC gateway listening")
	return s.Serve(lis)
}

func (g *GRPCGateway) buildGRPCBackends() map[string]string {
	out := make(map[string]string)
	for _, rt := range g.loader.Current().Routes {
		if rt.Protocol == "grpc" && rt.Enabled {
			out[rt.Path] = rt.UpstreamURL
		}
	}
	return out
}
