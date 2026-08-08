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
	"github.com/golang-jwt/jwt/v5"
	"time"
)

// custom JWT payload
type Claims struct {
	ClientID string   `json:"sub"`
	Role     string   `json:"role"`
	Routes   []string `json:"routes,omitempty"` //route IDs
	jwt.RegisteredClaims
}

type JWTValidator struct {
	secretKey []byte
}

func NewJWTValidator(secret string) *JWTValidator {
	return &JWTValidator{
		secretKey: []byte(secret)}
}

func (v *JWTValidator) Validate(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&Claims{},
		func(t *jwt.Token) (interface{}, error) { //Function to provide secret key

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

func (v *JWTValidator) ValidateIdentity(
	tokenStr string,
) (*Identity, error) {
	claims, err := v.Validate(
		tokenStr,
	)

	if err != nil {
		return nil, err
	}

	return &Identity{
		ClientID: claims.ClientID,
		Role:     claims.Role,
		Scopes:   []string{},
	}, nil
}
