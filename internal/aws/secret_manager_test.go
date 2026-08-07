package aws_test

import (
	"context"
	"testing"

	eliteaws "elitegate/internal/aws"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSecretsManagerAPI struct {
	createFn func(
		context.Context,
		*secretsmanager.CreateSecretInput,
		...func(*secretsmanager.Options),
	) (*secretsmanager.CreateSecretOutput, error)

	putFn func(
		context.Context,
		*secretsmanager.PutSecretValueInput,
		...func(*secretsmanager.Options),
	) (*secretsmanager.PutSecretValueOutput, error)

	getFn func(
		context.Context,
		*secretsmanager.GetSecretValueInput,
		...func(*secretsmanager.Options),
	) (*secretsmanager.GetSecretValueOutput, error)

	deleteFn func(
		context.Context,
		*secretsmanager.DeleteSecretInput,
		...func(*secretsmanager.Options),
	) (*secretsmanager.DeleteSecretOutput, error)

	restoreFn func(
		context.Context,
		*secretsmanager.RestoreSecretInput,
		...func(*secretsmanager.Options),
	) (*secretsmanager.RestoreSecretOutput, error)
}

func (m *mockSecretsManagerAPI) CreateSecret(
	ctx context.Context,
	input *secretsmanager.CreateSecretInput,
	optFns ...func(*secretsmanager.Options),
) (*secretsmanager.CreateSecretOutput, error) {
	return m.createFn(ctx, input, optFns...)
}

func (m *mockSecretsManagerAPI) PutSecretValue(
	ctx context.Context,
	input *secretsmanager.PutSecretValueInput,
	optFns ...func(*secretsmanager.Options),
) (*secretsmanager.PutSecretValueOutput, error) {
	return m.putFn(ctx, input, optFns...)
}

func (m *mockSecretsManagerAPI) GetSecretValue(
	ctx context.Context,
	input *secretsmanager.GetSecretValueInput,
	optFns ...func(*secretsmanager.Options),
) (*secretsmanager.GetSecretValueOutput, error) {
	return m.getFn(ctx, input, optFns...)
}

func (m *mockSecretsManagerAPI) DeleteSecret(
	ctx context.Context,
	input *secretsmanager.DeleteSecretInput,
	optFns ...func(*secretsmanager.Options),
) (*secretsmanager.DeleteSecretOutput, error) {
	return m.deleteFn(ctx, input, optFns...)
}

func (m *mockSecretsManagerAPI) RestoreSecret(
	ctx context.Context,
	input *secretsmanager.RestoreSecretInput,
	optFns ...func(*secretsmanager.Options),
) (*secretsmanager.RestoreSecretOutput, error) {
	return m.restoreFn(ctx, input, optFns...)
}

func TestAWSSecretManagerLifecycle(t *testing.T) {
	ctx := context.Background()

	mock := &mockSecretsManagerAPI{
		createFn: func(
			_ context.Context,
			input *secretsmanager.CreateSecretInput,
			_ ...func(*secretsmanager.Options),
		) (*secretsmanager.CreateSecretOutput, error) {
			assert.Equal(
				t,
				"elitegate/test/projects/project-a/jwt/hs256",
				awssdk.ToString(input.Name),
			)

			assert.Equal(
				t,
				"customer-secret",
				awssdk.ToString(input.SecretString),
			)

			return &secretsmanager.CreateSecretOutput{
				ARN:       awssdk.String("arn:aws:secretsmanager:test:secret:a"),
				VersionId: awssdk.String("version-1"),
			}, nil
		},

		putFn: func(
			_ context.Context,
			input *secretsmanager.PutSecretValueInput,
			_ ...func(*secretsmanager.Options),
		) (*secretsmanager.PutSecretValueOutput, error) {
			assert.Equal(
				t,
				"arn:aws:secretsmanager:test:secret:a",
				awssdk.ToString(input.SecretId),
			)

			return &secretsmanager.PutSecretValueOutput{
				ARN:       awssdk.String("arn:aws:secretsmanager:test:secret:a"),
				VersionId: awssdk.String("version-2"),
			}, nil
		},

		getFn: func(
			_ context.Context,
			input *secretsmanager.GetSecretValueInput,
			_ ...func(*secretsmanager.Options),
		) (*secretsmanager.GetSecretValueOutput, error) {
			assert.Equal(
				t,
				"version-2",
				awssdk.ToString(input.VersionId),
			)

			return &secretsmanager.GetSecretValueOutput{
				SecretString: awssdk.String("rotated-secret"),
			}, nil
		},

		deleteFn: func(
			_ context.Context,
			input *secretsmanager.DeleteSecretInput,
			_ ...func(*secretsmanager.Options),
		) (*secretsmanager.DeleteSecretOutput, error) {
			assert.Equal(
				t,
				"arn:aws:secretsmanager:test:secret:a",
				awssdk.ToString(input.SecretId),
			)

			assert.False(
				t,
				awssdk.ToBool(input.ForceDeleteWithoutRecovery),
			)

			return &secretsmanager.DeleteSecretOutput{}, nil
		},
	}

	manager := eliteaws.NewAWSSecretManagerWithAPI(mock)

	created, err := manager.CreateSecret(
		ctx,
		"elitegate/test/projects/project-a/jwt/hs256",
		"customer-secret",
	)
	require.NoError(t, err)

	assert.Equal(
		t,
		"arn:aws:secretsmanager:test:secret:a",
		created.ARN,
	)

	assert.Equal(t, "version-1", created.VersionID)

	updated, err := manager.UpdateSecret(
		ctx,
		created.ARN,
		"rotated-secret",
	)
	require.NoError(t, err)

	assert.Equal(t, "version-2", updated.VersionID)

	value, err := manager.GetSecret(
		ctx,
		created.ARN,
		updated.VersionID,
	)
	require.NoError(t, err)

	assert.Equal(t, "rotated-secret", value)

	require.NoError(
		t,
		manager.DeleteSecret(ctx, created.ARN),
	)
}

func TestAWSSecretManagerRejectsEmptySecret(t *testing.T) {
	manager := eliteaws.NewAWSSecretManagerWithAPI(
		&mockSecretsManagerAPI{},
	)

	_, err := manager.CreateSecret(
		context.Background(),
		"elitegate/test/project/test",
		"",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret value is required")
}
