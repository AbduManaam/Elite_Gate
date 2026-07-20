package auth_test

import (
	"testing"

	"elitegate/internal/auth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePasswordResetToken(t *testing.T) {
	token1, err := auth.GeneratePasswordResetToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token1)

	token2, err := auth.GeneratePasswordResetToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token2)

	assert.NotEqual(t, token1, token2, "generated reset tokens must be unique")

	hash1 := auth.HashToken(token1)
	hash2 := auth.HashToken(token1)
	assert.Equal(t, hash1, hash2, "token hashing must be deterministic")

	assert.NotContains(t, token1, "+", "URL safety check")
	assert.NotContains(t, token1, "/", "URL safety check")
	assert.NotContains(t, token1, "=", "URL safety check")
}
