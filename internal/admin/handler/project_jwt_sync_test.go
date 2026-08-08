package handler

import (
	"testing"

	"elitegate/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectJWTConfigToSyncEnabled(
	t *testing.T,
) {
	issuer :=
		"https://auth.example.com"

	cfg := &model.ProjectJWTConfig{
		Enabled: true,

		Algorithm: model.JWTAlgorithmHS256,

		SecretARN: "arn:aws:secretsmanager:ap-south-1:123456789012:secret:project-a",

		SecretVersionID: "version-2",

		ConfigVersion: 4,

		Issuer: &issuer,

		Audiences: []string{
			"api-a",
			"api-b",
		},

		SubjectClaim: "sub",
		RoleClaim:    "role",
		ScopesClaim:  "scope",

		ClockSkewSeconds: 30,
	}

	result, err :=
		projectJWTConfigToSync(cfg)

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(
		t,
		result.Enabled,
	)

	assert.Equal(
		t,
		model.JWTAlgorithmHS256,
		result.Algorithm,
	)

	assert.Equal(
		t,
		cfg.SecretARN,
		result.SecretARN,
	)

	assert.Equal(
		t,
		"version-2",
		result.SecretVersionID,
	)

	assert.Equal(
		t,
		int64(4),
		result.ConfigVersion,
	)

	assert.Equal(
		t,
		[]string{
			"api-a",
			"api-b",
		},
		result.Audiences,
	)
}

func TestProjectJWTConfigToSyncDisabledHidesSecretReference(
	t *testing.T,
) {
	cfg := &model.ProjectJWTConfig{
		Enabled: false,

		Algorithm: model.JWTAlgorithmHS256,

		SecretARN: "arn:aws:secretsmanager:should-not-be-sent",

		SecretVersionID: "should-not-be-sent",

		ConfigVersion: 7,

		Audiences: []string{},

		SubjectClaim: "sub",
		RoleClaim:    "role",
		ScopesClaim:  "scope",

		ClockSkewSeconds: 30,
	}

	result, err :=
		projectJWTConfigToSync(cfg)

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.False(
		t,
		result.Enabled,
	)

	assert.Empty(
		t,
		result.SecretARN,
	)

	assert.Empty(
		t,
		result.SecretVersionID,
	)

	assert.Equal(
		t,
		int64(7),
		result.ConfigVersion,
	)
}

func TestProjectJWTConfigToSyncRejectsBrokenEnabledConfig(
	t *testing.T,
) {
	cfg := &model.ProjectJWTConfig{
		Enabled: true,

		Algorithm: model.JWTAlgorithmHS256,

		ConfigVersion: 1,

		Audiences: []string{},

		SubjectClaim: "sub",
		RoleClaim:    "role",
		ScopesClaim:  "scope",

		ClockSkewSeconds: 30,
	}

	result, err :=
		projectJWTConfigToSync(cfg)

	require.Error(t, err)
	assert.Nil(t, result)
}

func TestProjectJWTConfigToSyncNil(
	t *testing.T,
) {
	result, err :=
		projectJWTConfigToSync(nil)

	require.NoError(t, err)
	assert.Nil(t, result)
}
