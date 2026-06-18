package gateway

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"elitegate/internal/gateway/health"
	"elitegate/internal/gateway/proxy"
	"elitegate/internal/gateway/runtime"
)

type GRPCGateway struct {
	logger       zerolog.Logger
	loader       *runtime.Loader
	port         string
	hostMap      map[string]string
	interceptors *GRPCSecurityInterceptors
	health       *health.Checker
	mu           sync.RWMutex
	backends     map[string]string
}

func NewGRPCGateway(logger zerolog.Logger, loader *runtime.Loader, port string, hostMap map[string]string, interceptors *GRPCSecurityInterceptors, hc *health.Checker) *GRPCGateway {
	g := &GRPCGateway{
		logger:       logger,
		loader:       loader,
		port:         port,
		hostMap:      hostMap,
		interceptors: interceptors,
		health:       hc,
	}
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

	// Periodically refresh backends from route loader snapshot
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				g.RefreshBackends()
			}
		}
	}()

	var opts []grpc.ServerOption
	opts = append(opts, grpc.ForceServerCodec(proxy.ProxyCodec{}))
	if g.interceptors != nil {
		opts = append(opts, grpc.UnaryInterceptor(g.interceptors.Unary()))
		opts = append(opts, grpc.StreamInterceptor(g.interceptors.Stream()))
	}
	opts = append(opts, grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
		//grpc.MethodFromServerStream is not public API.
		fullMethod, ok := grpc.Method(stream.Context())
		if !ok {
			return status.Error(codes.Internal, "method not found")
		}

		//read from cache instead of rebuilding on every RPC.
		g.mu.RLock()
		backends := g.backends
		g.mu.RUnlock()

		addr, ok := proxy.ResolveGRPCBackend(fullMethod, backends)
		if !ok {
			return status.Error(codes.NotFound, "no grpc route")
		}

		// HEALTH CHECK
		if g.health != nil && !g.health.IsHealthy(addr) {
			return status.Error(codes.Unavailable, "upstream is currently unhealthy")
		}

		return proxy.TransparentHandler(addr)(srv, stream)
	}))

	s := grpc.NewServer(opts...)

	//graceful shutdown when ctx is cancelled.
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
			out[rt.Path] = proxy.RewriteTargetURL(rt.UpstreamURL, g.hostMap)
		}
	}
	return out
}
