package gateway

import (
	"context"
	"elitegate/internal/gateway/middleware"
	"elitegate/internal/gateway/runtime"
	"elitegate/internal/ipfilter"
	"elitegate/internal/model"
	"elitegate/internal/ratelimit"
	"elitegate/internal/shared"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type AuthInfo struct {
	ClientID string
	Role     string
}

type GRPCSecurityInterceptors struct {
	loader         *runtime.Loader
	authMiddleware *middleware.AuthMiddleware
	limiter        ratelimit.Limiter
	trustProxy     bool
	logger         zerolog.Logger
}

func NewGRPCSecurityInterceptors(
	loader *runtime.Loader,
	authMiddleware *middleware.AuthMiddleware,
	limiter ratelimit.Limiter,
	trustProxy bool,
	logger zerolog.Logger,

) *GRPCSecurityInterceptors {
	return &GRPCSecurityInterceptors{
		loader:         loader,
		authMiddleware: authMiddleware,
		limiter:        limiter,
		trustProxy:     trustProxy,
		logger:         logger,
	}
}

func (in *GRPCSecurityInterceptors) getClientIP(ctx context.Context) string {
	var remoteAddr string
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		remoteAddr = p.Addr.String()
	} else {
		in.logger.Warn().Msg("getClientIP: could not extract peer address from context")
	}

	getHeader := func(key string) string {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(key); len(vals) > 0 {
				return vals[0]
			}
		}
		return ""
	}

	ip := ipfilter.ExtractIP(remoteAddr, getHeader, in.trustProxy)
	if ip == "" {
		in.logger.Warn().Msg("getClientIP: resolved IP is empty")
	}
	return ip
}

func (in *GRPCSecurityInterceptors) matchRoute(fullMethod string, config runtime.Snapshot) *model.Route {
	service := fullMethod
	if i := strings.LastIndex(service, "/"); i > 0 {
		service = service[:i]
	}

	var best *model.Route
	bestLen := -1
	for _, rt := range config.Routes {
		if rt.Protocol == "grpc" && rt.Enabled && strings.HasPrefix(service, rt.Path) && len(rt.Path) > bestLen {
			// Fix: copy loop variable before taking address
			copied := rt
			best = &copied
			bestLen = len(rt.Path)
		}
	}
	return best
}

// Verifies that incoming gRPC requests are authenticated
func (in *GRPCSecurityInterceptors) resolveAuth(ctx context.Context, fullMethod string) (AuthInfo, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		in.logger.Warn().Str("method", fullMethod).Msg("auth: no metadata in context")
		return AuthInfo{}, status.Error(codes.Unauthenticated, "authentication required")
	}

	authVals := md.Get("authorization")
	apiKeyVals := md.Get("x-api-key")

	if len(authVals) > 0 && strings.HasPrefix(authVals[0], "Bearer ") {
		token := strings.TrimPrefix(authVals[0], "Bearer ")
		claims, err := in.authMiddleware.JWTValidator.Validate(token)
		if err != nil {
			// Log internally, never expose err details to caller
			in.logger.Warn().
				Err(err).
				Str("method", fullMethod).
				Msg("auth: JWT validation failed")
			return AuthInfo{}, status.Error(codes.Unauthenticated, "invalid token")
		}
		in.logger.Debug().
			Str("client_id", claims.ClientID).
			Str("role", claims.Role).
			Str("method", fullMethod).
			Msg("auth: JWT validated successfully")
		return AuthInfo{ClientID: claims.ClientID, Role: claims.Role}, nil
	}

	if len(apiKeyVals) > 0 && in.authMiddleware.KeyStore != nil {
		id, valid := in.authMiddleware.KeyStore.Validate(apiKeyVals[0])
		if !valid {
			in.logger.Warn().
				Str("method", fullMethod).
				Msg("auth: API key validation failed")
			return AuthInfo{}, status.Error(codes.Unauthenticated, "invalid api key")
		}
		in.logger.Debug().
			Str("client_id", id).
			Str("method", fullMethod).
			Msg("auth: API key validated successfully")
		return AuthInfo{ClientID: id, Role: "client"}, nil
	}

	in.logger.Warn().
		Str("method", fullMethod).
		Msg("auth: no valid credentials provided")
	return AuthInfo{}, status.Error(codes.Unauthenticated, "authentication required")
}

// it does:-
// Uses an atomic route snapshot to avoid config changes during requests
// Finds the matching route using matchRoute() or returns 404
// Handles required authentication and passes auth info through the request context
// Applies rate limits and returns 429 when the limit is exceeded.

func (in *GRPCSecurityInterceptors) validate(ctx context.Context, fullMethod string) (context.Context, error) {
	clientIP := in.getClientIP(ctx)

	// Snapshot config once to avoid inconsistency during hot-reload
	config := in.loader.Current()

	// 1. Route Matching
	rt := in.matchRoute(fullMethod, config)
	if rt == nil {
		in.logger.Warn().
			Str("method", fullMethod).
			Str("ip", clientIP).
			Msg("validate: no matching route found")
		return nil, status.Error(codes.NotFound, "route not found")
	}
	ctx = context.WithValue(ctx, shared.ContextKeyRoute, rt)

	// 2. Authentication
	authInfo := AuthInfo{Role: "anonymous"}
	if rt.AuthRequired {
		var err error
		authInfo, err = in.resolveAuth(ctx, fullMethod)
		if err != nil {
			return nil, err
		}
	}
	// Always store auth info so downstream handlers can always read it safely
	ctx = context.WithValue(ctx, shared.ContextKeyAuthInfo, authInfo)

	// 3. Rate Limiting
	if rt.RateLimitRPM > 0 {
		keyID := authInfo.ClientID
		if keyID == "" {
			keyID = clientIP
		}
		rateKey := fmt.Sprintf("%s:%s", keyID, fullMethod)
		if !in.limiter.AllowWithLimit(rateKey, rt.RateLimitRPM) {
			in.logger.Warn().
				Str("ip", clientIP).
				Str("method", fullMethod).
				Str("key_id", keyID).
				Int("limit_rpm", rt.RateLimitRPM).
				Msg("validate: rate limit exceeded")
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
	}

	in.logger.Debug().
		Str("ip", clientIP).
		Str("method", fullMethod).
		Str("client_id", authInfo.ClientID).
		Str("role", authInfo.Role).
		Msg("validate: request passed all checks")

	return ctx, nil
}

// Runs authentication and authorization before executing each unary gRPC request
func (in *GRPCSecurityInterceptors) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		newCtx, err := in.validate(ctx, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return handler(newCtx, req)
	}
}

// Runs authentication and authorization for streaming gRPC requests
func (in *GRPCSecurityInterceptors) Stream() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		newCtx, err := in.validate(ss.Context(), info.FullMethod)
		if err != nil {
			return err
		}
		return handler(srv, &wrappedServerStream{ServerStream: ss, ctx: newCtx})
	}
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
