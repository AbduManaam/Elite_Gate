package middleware

import (
	"context"
	"net/http"

	"elitegate/internal/gateway/router"
	"elitegate/internal/gateway/runtime"
	"elitegate/internal/shared"
)

func RouteMatcher(loader *runtime.Loader) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			snap := loader.Current()
			rt := router.MatchHttP(r.URL.Path, snap.Routes)
			if rt == nil {
				httpJSON(w, http.StatusNotFound, map[string]string{
					"error": "route not found",
				})
				return
			}

			// Stores the matched route object (rt) inside the context with a specific key
			ctx := context.WithValue(r.Context(), shared.ContextKeyRoute, rt)
			//  Updates the HTTP request to carry this new context, and forwards it
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
