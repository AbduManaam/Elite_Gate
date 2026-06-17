package middleware

import (
	"net/http"
	"strings"

	"elitegate/internal/model"
	"elitegate/internal/shared"
)

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt, ok := r.Context().Value(shared.ContextKeyRoute).(*model.Route)
		if !ok || rt == nil || len(rt.AllowedOrigins) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		// Always vary on Origin to prevent cache poisoning
		w.Header().Add("Vary", "Origin")

		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		wildcardAllowed := false
		originAllowed := false
		for _, o := range rt.AllowedOrigins {
			if o == "*" {
				wildcardAllowed = true
				break
			}
			if o == origin {
				originAllowed = true
				break
			}
		}

		if !wildcardAllowed && !originAllowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"CORS origin not allowed"}`))
			return
		}

		// Wildcard + credentials is invalid per spec
		if wildcardAllowed {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == http.MethodOptions {
			methods := "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD"
			if len(rt.Methods) > 0 {
				hasAny := false
				for _, m := range rt.Methods {
					if strings.EqualFold(m, "ANY") {
						hasAny = true
						break
					}
				}
				if !hasAny {
					methods = strings.Join(rt.Methods, ", ")
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", methods)

			// Dynamically allow requested headers or fallback to defaults
			allowedHeaders := "Content-Type, Authorization, X-API-Key"
			if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
				allowedHeaders = reqHeaders
			}
			w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)

			w.Header().Set("Access-Control-Max-Age", "7200")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
