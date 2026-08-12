package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

const (
	EmailVerificationTokenBytes = 32
	EmailVerificationTokenTTL   = 30 * time.Minute
)

func GenerateEmailVerificationToken() (string, error) {
	tokenBytes := make([]byte, EmailVerificationTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate email verification token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(tokenBytes), nil
}
