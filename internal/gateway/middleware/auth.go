package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"elitegate/helper"
	"elitegate/internal/auth"
	"elitegate/internal/model"
	"elitegate/internal/shared"

	"github.com/rs/zerolog"
)

type APIKeyStore interface {
	Validate(key string) (*auth.APIKeyRecord, bool)
}

type JWTValidator interface {
	Validate(
		token string,
	) (*auth.Identity, error)
}

type AuthMiddleware struct {
	JWTValidator JWTValidator
	KeyStore     APIKeyStore
	Logger       *zerolog.Logger
}

func NewAuthMiddleware(
	jwtValidator JWTValidator,
	keyStore APIKeyStore,
	logger *zerolog.Logger,
) *AuthMiddleware {
	return &AuthMiddleware{
		JWTValidator: jwtValidator,
		KeyStore:     keyStore,
		Logger:       logger,
	}
}

func (a *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) || isPreflight(r) {
			next.ServeHTTP(w, r)
			return
		}
		rt, ok := r.Context().Value(shared.ContextKeyRoute).(*model.Route)
		if ok && rt != nil && !rt.AuthRequired {
			next.ServeHTTP(w, r)
			return
		}

		var clientID, role string
		var scopes []string

		if bearer := r.Header.Get("Authorization"); strings.HasPrefix(bearer, "Bearer ") {
			if a.JWTValidator == nil {
				httpJSON(
					w,
					http.StatusUnauthorized,
					map[string]string{
						"error": "JWT authentication is not configured",
					},
				)
				return
			}

			tokenStr := strings.TrimPrefix(
				bearer,
				"Bearer ",
			)

			identity, err := a.JWTValidator.Validate(
				tokenStr,
			)
			if err != nil {
				if a.Logger != nil {
					a.Logger.Warn().
						Err(err).
						Str("path", r.URL.Path).
						Msg("JWT validation failed")
				}

				// Do NOT expose validation internals.
				httpJSON(
					w,
					http.StatusUnauthorized,
					map[string]string{
						"error": "invalid token",
					},
				)
				return
			}

			clientID = identity.ClientID
			role = identity.Role
			scopes = identity.Scopes
		} else if key := strings.TrimSpace(r.Header.Get("X-API-Key")); key != "" && a.KeyStore != nil {
			rec, valid := a.KeyStore.Validate(key)
			if !valid {
				httpJSON(w, http.StatusUnauthorized,
					map[string]string{"error": "invalid api key"})
				return
			}
			clientID = rec.ClientID
			role = "client"
			scopes = rec.Scopes
		} else {
			httpJSON(w, http.StatusUnauthorized,
				map[string]string{"error": "authentication required"})
			return
		}

		// ── ACL enforcement ──────────────────────────────────────────────────────
		if rt != nil {
			// Role check: if the route restricts roles, client must have one of them
			if len(rt.AllowedRoles) > 0 && !helper.Contains(rt.AllowedRoles, role) {
				a.Logger.Warn().
					Str("path", r.URL.Path).
					Str("client_id", clientID).
					Str("client_role", role).
					Strs("allowed_roles", rt.AllowedRoles).
					Msg("authz: role not permitted")
				httpJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: role not permitted"})
				return
			}
			// Scope check: client must have ALL required scopes
			if len(rt.AllowedScopes) > 0 && !helper.HasAllScopes(scopes, rt.AllowedScopes) {
				a.Logger.Warn().
					Str("path", r.URL.Path).
					Str("client_id", clientID).
					Strs("client_scopes", scopes).
					Strs("required_scopes", rt.AllowedScopes).
					Msg("authz: missing required scopes")
				httpJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: insufficient scopes"})
				return
			}
		}

		ctx := context.WithValue(r.Context(), shared.ContextKeyClientID, clientID)
		ctx = context.WithValue(ctx, shared.ContextKeyRole, role)
		ctx = context.WithValue(ctx, shared.ContextKeyScopes, scopes)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isPublicPath(path string) bool {
	return path == "/health" || path == "/ready"
}

func httpJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
