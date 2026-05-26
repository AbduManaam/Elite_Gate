package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"elitegate/internal/auth"
	"elitegate/internal/shared"
)

func Auth(next http.Handler) http.Handler {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "super-secret-key"
	}

	validator := auth.NewJWTValidator(secret)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := validator.Validate(tokenStr)
			if err != nil {
				httpJSON(w, http.StatusUnauthorized, map[string]string{
					"error":  "invalid token",
					"detail": err.Error(),
				})
				return
			}

			ctx := context.WithValue(r.Context(), shared.ContextKeyClientID, claims.ClientID)
			ctx = context.WithValue(ctx, shared.ContextKeyRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if apiKey := r.Header.Get("X-API-Key"); apiKey == "test-key" {
			ctx := context.WithValue(r.Context(), shared.ContextKeyClientID, "test-client")
			ctx = context.WithValue(ctx, shared.ContextKeyRole, "client")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		httpJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	})
}

// Responsible for:

// reading Authorization header
// calling JWT validator
// attaching identity into request context
// blocking invalid requests
func (a *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		var clientID, role string

		if bearer := r.Header.Get("Authorization"); strings.HasPrefix(bearer, "Bearer ") {
			tokenStr := strings.TrimPrefix(bearer, "Bearer ")
			claims, err := a.jwtValidator.Validate(tokenStr)
			if err != nil {
				httpJSON(w, http.StatusUnauthorized,
					map[string]string{"error": "invalid token", "detail": err.Error()})
				return
			}
			clientID = claims.ClientID
			role = claims.Role
		} else if key := r.Header.Get("X-API-Key"); key != "" && a.keyStore != nil {
			id, valid := a.keyStore.Validate(key)
			if !valid {
				httpJSON(w, http.StatusUnauthorized,
					map[string]string{"error": "invalid api key"})
				return
			}
			clientID = id
			role = "client"
		} else {
			httpJSON(w, http.StatusUnauthorized,
				map[string]string{"error": "authentication required"})
			return
		}

		ctx := context.WithValue(r.Context(), shared.ContextKeyClientID, clientID)
		ctx = context.WithValue(ctx, shared.ContextKeyRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type APIKeyStore interface {
	Validate(key string) (string, bool)
}

type AuthMiddleware struct {
	jwtValidator *auth.JWTValidator
	keyStore     APIKeyStore
}

func NewAuthMiddleware(jwtValidator *auth.JWTValidator, keyStore APIKeyStore) *AuthMiddleware {
	return &AuthMiddleware{
		jwtValidator: jwtValidator,
		keyStore:     keyStore,
	}
}

func isPublicPath(path string) bool {
	return path == "/health" || path == "/ready"
}

func httpJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
