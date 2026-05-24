package auth

import (
	"context"
	"edgecore/internal/shared"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the custom JWT payload for EdgeCore.

type Claims struct {
	ClientID string `json:"sub"`
	Role string `json:"role"`
	Routes []string `json:"routes,omitempty"` // allowed route IDs
	jwt.RegisteredClaims
}

type JWTValidator struct {
	secretKey []byte
}

func NewJWTValidator(secret string) *JWTValidator {
	return &JWTValidator{secretKey: []byte(secret)}
}

// Validate parses + validates a raw JWT string.
// Returns Claims on success, error on any failure.

func (v *JWTValidator) Validate(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
	tokenStr,
	&Claims{},
func(t *jwt.Token) (interface{}, error) {

// Reject tokens that use unexpected algorithms
	if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
	return nil, fmt.Errorf("unexpected signing method: %v",
	t.Header["alg"])
}
return v.secretKey, nil
},
)

if err != nil {
	return nil, fmt.Errorf("jwt.Validate: %w", err)
}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
	return nil, errors.New("jwt.Validate: invalid token")
}
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
	return nil, errors.New("jwt.Validate: token expired")
}
    return claims, nil
}


func (a *AuthMiddleware) Middleware(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if isPublicPath(r.URL.Path) {
	next.ServeHTTP(w, r); return
}

// Determine auth method from route config (stored in context by router)
// For now, support both JWT and API key in the same middleware

var clientID, role string

// Try JWT first
	if bearer := r.Header.Get("Authorization"); bearer != "" {
	tokenStr := strings.TrimPrefix(bearer, "Bearer ")
	claims, err := a.jwtValidator.Validate(tokenStr)

if err != nil {
	httpJSON(w, http.StatusUnauthorized,
	map[string]string{"error": "invalid token", "detail": err.Error()})
	return
}
	clientID = claims.ClientID
	role = claims.Role
// Try API key second
	} else if key := r.Header.Get("X-API-Key"); key != "" {
	id, valid := a.keyStore.Validate(key)

if !valid {
	httpJSON(w, http.StatusUnauthorized,
	map[string]string{"error": "invalid api key"})
	return
}
clientID = id
role = "client" // default role for API keys
} else {
	httpJSON(w, http.StatusUnauthorized,
	map[string]string{"error": "authentication required"})
	return
}

// Attach identity to context for downstream middleware
	ctx := context.WithValue(r.Context(), shared.ContextKeyClientID, clientID)
	ctx = context.WithValue(ctx, shared.ContextKeyRole, role)
	next.ServeHTTP(w, r.WithContext(ctx))
})
}