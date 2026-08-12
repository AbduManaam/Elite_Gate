package auth_test

import (
	"encoding/base64"
	"testing"
	"time"

	"elitegate/internal/auth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailVerificationTokenTTL(t *testing.T) {
	assert.Equal(t, 30*time.Minute, auth.EmailVerificationTokenTTL, "EmailVerificationTokenTTL must be exactly 30 minutes")
}

func TestGenerateEmailVerificationToken(t *testing.T) {
	token1, err := auth.GenerateEmailVerificationToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token1)

	token2, err := auth.GenerateEmailVerificationToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token2)

	assert.NotEqual(t, token1, token2, "generated email verification tokens must be unique")

	decoded, err := base64.RawURLEncoding.DecodeString(token1)
	require.NoError(t, err, "token must be valid base64.RawURLEncoding")
	assert.Len(t, decoded, auth.EmailVerificationTokenBytes, "decoded token must contain exactly 32 bytes")

	assert.NotContains(t, token1, "+", "URL safety check: token must not contain '+'")
	assert.NotContains(t, token1, "/", "URL safety check: token must not contain '/'")
	assert.NotContains(t, token1, "=", "URL safety check: token must not contain '='")

	hash1 := auth.HashToken(token1)
	hash2 := auth.HashToken(token1)
	assert.Equal(t, hash1, hash2, "token hashing must be deterministic")
	assert.NotEqual(t, token1, hash1, "hash must differ from raw token")
}
