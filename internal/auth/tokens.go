package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	AdminRole              = "admin"
	AccessTokenTTL         = 15 * time.Minute
	RefreshTokenTTL        = 7 * 24 * time.Hour
	MinJWTSecretByteLength = 32
)

type AdminClaims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type AdminTokenManager struct {
	secret []byte
	issuer string
}

func NewAdminTokenManager(secret,issuer string,) (*AdminTokenManager, error) {

	if len([]byte(secret)) < MinJWTSecretByteLength {
		return nil, fmt.Errorf(
			"JWT_SECRET must be at least %d bytes",
			MinJWTSecretByteLength,
		)
	}

	if issuer == "" {
		issuer = "elitegate-admin"
	}

	return &AdminTokenManager{
		secret: []byte(secret),
		issuer: issuer,
	}, nil
}

func (m *AdminTokenManager) CreateAdminAccessToken(
	userID,
	username string,
) (string, error) {

	now := time.Now().UTC()

	claims := AdminClaims{
		Username: username,
		Role:     AdminRole,

		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID,
			Issuer:  m.issuer,

			IssuedAt: jwt.NewNumericDate(now),

			ExpiresAt: jwt.NewNumericDate(
				now.Add(AccessTokenTTL),
			),
		},
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	return token.SignedString(m.secret)
}

func (m *AdminTokenManager) ValidateAdminAccessToken(raw string,) (*AdminClaims, error) {

	token, err := jwt.ParseWithClaims(
		raw,
		&AdminClaims{},
		func(t *jwt.Token) (interface{}, error) {

			if t.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf(
					"unexpected signing method: %v",
					t.Header["alg"],
				)
			}

			return m.secret, nil
		},
	)

	if err != nil {

		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}

		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*AdminClaims)

	if !ok ||
		!token.Valid ||
		claims.Role != AdminRole ||
		claims.Subject == "" {

		return nil, ErrInvalidToken
	}

	return claims, nil
}

func GenerateRefreshToken() (string, error) {

	b := make([]byte, 32)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(raw string) string {

	sum := sha256.Sum256([]byte(raw))

	return hex.EncodeToString(sum[:])
}