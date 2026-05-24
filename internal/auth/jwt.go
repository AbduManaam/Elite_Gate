package auth

// Core JWT engine.

// Responsible for:

// parsing token
// validating signature
// checking expiry
// extracting claims
import (
 "errors"
 "fmt"
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