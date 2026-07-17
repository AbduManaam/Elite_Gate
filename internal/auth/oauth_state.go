package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const oauthStateTTL = 10 * time.Minute

// oauthStateClaims is a short-lived, signed token that round-trips through
// Google as the `state` parameter. It proves the callback belongs to a flow
// this server actually started (CSRF protection) and carries the PKCE
// code_verifier so we don't need server-side session storage.
type oauthStateClaims struct {
	CodeVerifier string `json:"cv"`
	RedirectPath string `json:"rp,omitempty"` // optional: where to send the user after login
	jwt.RegisteredClaims
}

type OAuthStateManager struct {
	secret []byte
}

func NewOAuthStateManager(secret string) *OAuthStateManager {
	return &OAuthStateManager{secret: []byte(secret)}
}

// GeneratePKCE returns a random code_verifier and its S256 code_challenge,
// per RFC 7636.
func GeneratePKCEPair() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// CreateState signs a new state token embedding the PKCE verifier.
func (m *OAuthStateManager) CreateState(codeVerifier string) (string, error) {
	claims := oauthStateClaims{
		CodeVerifier: codeVerifier,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(oauthStateTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// ValidateState verifies signature + expiry and returns the embedded verifier.
func (m *OAuthStateManager) ValidateState(raw string) (codeVerifier string, err error) {
	token, err := jwt.ParseWithClaims(raw, &oauthStateClaims{}, func(t *jwt.Token) (interface{}, error) {
		return m.secret, nil
	})
	if err != nil {
		return "", errors.New("invalid or expired oauth state")
	}
	claims, ok := token.Claims.(*oauthStateClaims)
	if !ok || !token.Valid {
		return "", errors.New("invalid oauth state")
	}
	return claims.CodeVerifier, nil
}
