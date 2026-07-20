package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const PasswordResetTokenBytes = 32

func GeneratePasswordResetToken() (string, error) {
	tokenBytes := make([]byte, PasswordResetTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate password reset token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(tokenBytes), nil
}
