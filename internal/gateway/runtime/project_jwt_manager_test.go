package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"elitegate/internal/model"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const managerJWTSecret = "12345678901234567890123456789012"

type fakeProjectJWTSecretReader struct {
	values map[string]string

	err error

	getCalls int
}

func (f *fakeProjectJWTSecretReader) GetSecret(
	_ context.Context,
	secretID string,
	versionID string,
) (string, error) {
	f.getCalls++

	if f.err != nil {
		return "", f.err
	}

	return f.values[secretID+"|"+versionID], nil
}

func managerJWTConfig() *model.ProjectJWTConfigSync {
	issuer := "https://auth.example.com"

	return &model.ProjectJWTConfigSync{
		Enabled: true,

		Algorithm: "HS256",

		SecretARN: "arn:test:secret",

		SecretVersionID: "version-1",

		ConfigVersion: 1,

		Issuer: &issuer,

		Audiences: []string{
			"api",
		},

		SubjectClaim: "sub",
		RoleClaim:    "role",
		ScopesClaim:  "scope",

		ClockSkewSeconds: 30,
	}
}

func TestProjectJWTManagerDoesNotReloadSameSecret(
	t *testing.T,
) {
	reader := &fakeProjectJWTSecretReader{
		values: map[string]string{
			"arn:test:secret|version-1": managerJWTSecret,
		},
	}

	manager := NewProjectJWTManager(
		reader,
		zerolog.Nop(),
	)

	cfg := managerJWTConfig()

	require.NoError(
		t,
		manager.Apply(
			context.Background(),
			cfg,
		),
	)

	require.Equal(
		t,
		1,
		reader.getCalls,
	)

	require.NoError(
		t,
		manager.Apply(
			context.Background(),
			cfg,
		),
	)

	// Same configuration: no second AWS call.
	assert.Equal(
		t,
		1,
		reader.getCalls,
	)
}

func TestProjectJWTManagerNonSecretUpdateAvoidsAWS(
	t *testing.T,
) {
	reader := &fakeProjectJWTSecretReader{
		values: map[string]string{
			"arn:test:secret|version-1": managerJWTSecret,
		},
	}

	manager := NewProjectJWTManager(
		reader,
		zerolog.Nop(),
	)

	cfg := managerJWTConfig()

	require.NoError(
		t,
		manager.Apply(
			context.Background(),
			cfg,
		),
	)

	cfg.ConfigVersion = 2

	issuer := "https://new-auth.example.com"

	cfg.Issuer = &issuer

	require.NoError(
		t,
		manager.Apply(
			context.Background(),
			cfg,
		),
	)

	assert.Equal(
		t,
		1,
		reader.getCalls,
	)
}

func TestProjectJWTManagerSecretRotationLoadsAWSOnce(
	t *testing.T,
) {
	reader := &fakeProjectJWTSecretReader{
		values: map[string]string{
			"arn:test:secret|version-1": managerJWTSecret,

			"arn:test:secret|version-2": "abcdefghijklmnopqrstuvwxyz123456",
		},
	}

	manager := NewProjectJWTManager(
		reader,
		zerolog.Nop(),
	)

	cfg := managerJWTConfig()

	require.NoError(
		t,
		manager.Apply(
			context.Background(),
			cfg,
		),
	)

	cfg.ConfigVersion = 2
	cfg.SecretVersionID = "version-2"

	require.NoError(
		t,
		manager.Apply(
			context.Background(),
			cfg,
		),
	)

	assert.Equal(
		t,
		2,
		reader.getCalls,
	)
}

func TestProjectJWTManagerFailedRotationKeepsOldVerifier(
	t *testing.T,
) {
	reader := &fakeProjectJWTSecretReader{
		values: map[string]string{
			"arn:test:secret|version-1": managerJWTSecret,
		},
	}

	manager := NewProjectJWTManager(
		reader,
		zerolog.Nop(),
	)

	cfg := managerJWTConfig()

	require.NoError(
		t,
		manager.Apply(
			context.Background(),
			cfg,
		),
	)

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		jwt.MapClaims{
			"sub": "user-1",

			"iss": "https://auth.example.com",

			"aud": "api",

			"exp": time.Now().
				Add(time.Hour).
				Unix(),
		},
	)

	signed, err := token.SignedString(
		[]byte(
			managerJWTSecret,
		),
	)

	require.NoError(t, err)

	// Break the next AWS lookup.
	reader.err = errors.New(
		"Secrets Manager unavailable",
	)

	cfg.ConfigVersion = 2
	cfg.SecretVersionID = "version-2"

	require.Error(
		t,
		manager.Apply(
			context.Background(),
			cfg,
		),
	)

	// Previous verifier remains active.
	identity, err := manager.ValidateIdentity(
		signed,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		"user-1",
		identity.ClientID,
	)
}

func TestProjectJWTManagerDisableClearsVerifier(
	t *testing.T,
) {
	reader := &fakeProjectJWTSecretReader{
		values: map[string]string{
			"arn:test:secret|version-1": managerJWTSecret,
		},
	}

	manager := NewProjectJWTManager(
		reader,
		zerolog.Nop(),
	)

	cfg := managerJWTConfig()

	require.NoError(
		t,
		manager.Apply(
			context.Background(),
			cfg,
		),
	)

	cfg.Enabled = false
	cfg.ConfigVersion = 2

	require.NoError(
		t,
		manager.Apply(
			context.Background(),
			cfg,
		),
	)

	_, err := manager.ValidateIdentity(
		"anything",
	)

	require.Error(t, err)
}
