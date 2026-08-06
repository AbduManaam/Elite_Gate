package middleware

import (
	"context"
	"net/http"
	"strings"

	"elitegate/internal/gateway/router"
	"elitegate/internal/gateway/runtime"
	"elitegate/internal/shared"
)

func isPreflight(r *http.Request) bool {
	if r.Method != http.MethodOptions {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	acrm := strings.TrimSpace(r.Header.Get("Access-Control-Request-Method"))
	return origin != "" && acrm != ""
}

func RouteMatcher(loader *runtime.Loader) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			snap := loader.Current()

			// r.Host is Go's primary source for HTTP host resolution (parsed natively from URL or Host header).
			// r.Header.Get("Host") is checked as a fallback for custom HTTP handlers.
			host := r.Host
			if host == "" {
				host = r.Header.Get("Host")
			}

			req := r
			if domainCtx, ok := snap.LookupDomain(host); ok {
				ctx := context.WithValue(r.Context(), shared.ContextKeyCustomDomain, domainCtx)
				req = r.WithContext(ctx)
			}

			methodToMatch := req.Method
			if isPreflight(req) {
				if acrm := strings.ToUpper(strings.TrimSpace(req.Header.Get("Access-Control-Request-Method"))); acrm != "" {
					methodToMatch = acrm
				}
			}

			rt := router.MatchHTTP(req.URL.Path, methodToMatch, snap.Routes)
			if rt == nil {
				httpJSON(w, http.StatusNotFound, map[string]string{
					"error": "route not found",
				})
				return
			}

			// Stores the matched route object (rt) inside the context with a specific key
			ctx := context.WithValue(req.Context(), shared.ContextKeyRoute, rt)
			// Updates the HTTP request to carry this new context, and forwards it
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}
