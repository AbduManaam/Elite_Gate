package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretsTypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

const defaultSecretOperationTimeout = 5 * time.Second

// SecretsManagerAPI contains only the AWS SDK methods EliteGate needs.
// Keeping this interface small makes the implementation easy to mock.
type SecretsManagerAPI interface {
	CreateSecret(
		ctx context.Context,
		params *secretsmanager.CreateSecretInput,
		optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.CreateSecretOutput, error)

	PutSecretValue(
		ctx context.Context,
		params *secretsmanager.PutSecretValueInput,
		optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.PutSecretValueOutput, error)

	GetSecretValue(
		ctx context.Context,
		params *secretsmanager.GetSecretValueInput,
		optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.GetSecretValueOutput, error)

	DeleteSecret(
		ctx context.Context,
		params *secretsmanager.DeleteSecretInput,
		optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.DeleteSecretOutput, error)

	RestoreSecret(
		ctx context.Context,
		params *secretsmanager.RestoreSecretInput,
		optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.RestoreSecretOutput, error)
}

// AWSSecretManager is EliteGate's AWS Secrets Manager adapter.
//
// It contains no project/database logic. Its only responsibility is secure
// interaction with AWS Secrets Manager.
type AWSSecretManager struct {
	client  SecretsManagerAPI
	timeout time.Duration
}

// NewAWSSecretManager initializes Secrets Manager using AWS's default
// credential chain. In production this resolves to the EC2 IAM role.
func NewAWSSecretManager(
	ctx context.Context,
	region string,
) (*AWSSecretManager, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return nil, errors.New("AWS region is required")
	}

	sdkCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK config for Secrets Manager: %w", err)
	}

	return &AWSSecretManager{
		client:  secretsmanager.NewFromConfig(sdkCfg),
		timeout: defaultSecretOperationTimeout,
	}, nil
}

// NewAWSSecretManagerWithAPI is used for unit tests and dependency injection.
func NewAWSSecretManagerWithAPI(
	client SecretsManagerAPI,
) *AWSSecretManager {
	return &AWSSecretManager{
		client:  client,
		timeout: defaultSecretOperationTimeout,
	}
}

func (m *AWSSecretManager) CreateSecret(
	ctx context.Context,
	name string,
	secretValue string,
) (*SecretReference, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("secret name is required")
	}

	if secretValue == "" {
		return nil, errors.New("secret value is required")
	}

	opCtx, cancel := m.operationContext(ctx)
	defer cancel()

	output, err := m.client.CreateSecret(
		opCtx,
		&secretsmanager.CreateSecretInput{
			Name:         awssdk.String(name),
			SecretString: awssdk.String(secretValue),
		},
	)
	if err != nil {
		var existsErr *secretsTypes.ResourceExistsException
		if errors.As(err, &existsErr) {
			return nil, fmt.Errorf(
				"%w: %v",
				ErrSecretAlreadyExists,
				err,
			)
		}

		// AWS returns InvalidRequestException instead of ResourceExistsException
		// when a secret with this name exists but is scheduled for deletion.
		// Treat that state as an existing secret so the service can restore it
		// and rotate its value.
		var invalidRequestErr *secretsTypes.InvalidRequestException
		if errors.As(err, &invalidRequestErr) &&
			strings.Contains(
				strings.ToLower(err.Error()),
				"scheduled for deletion",
			) {
			return nil, fmt.Errorf(
				"%w: %v",
				ErrSecretAlreadyExists,
				err,
			)
		}

		return nil, fmt.Errorf("create secret: %w", err)
	}

	ref, err := secretReference(
		output.ARN,
		output.VersionId,
	)
	if err != nil {
		return nil, fmt.Errorf("create secret response: %w", err)
	}

	return ref, nil
}

func (m *AWSSecretManager) UpdateSecret(
	ctx context.Context,
	secretID string,
	secretValue string,
) (*SecretReference, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}

	secretID = strings.TrimSpace(secretID)
	if secretID == "" {
		return nil, errors.New("secret ID is required")
	}

	if secretValue == "" {
		return nil, errors.New("secret value is required")
	}

	opCtx, cancel := m.operationContext(ctx)
	defer cancel()

	output, err := m.client.PutSecretValue(
		opCtx,
		&secretsmanager.PutSecretValueInput{
			SecretId:     awssdk.String(secretID),
			SecretString: awssdk.String(secretValue),
		},
	)
	if err != nil {
		var notFoundErr *secretsTypes.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return nil, fmt.Errorf("secret not found: %w", err)
		}

		return nil, fmt.Errorf("update secret: %w", err)
	}

	ref, err := secretReference(
		output.ARN,
		output.VersionId,
	)
	if err != nil {
		return nil, fmt.Errorf("update secret response: %w", err)
	}

	return ref, nil
}

func (m *AWSSecretManager) GetSecret(
	ctx context.Context,
	secretID string,
	versionID string,
) (string, error) {
	if err := m.validate(); err != nil {
		return "", err
	}

	secretID = strings.TrimSpace(secretID)
	if secretID == "" {
		return "", errors.New("secret ID is required")
	}

	input := &secretsmanager.GetSecretValueInput{
		SecretId: awssdk.String(secretID),
	}

	if versionID = strings.TrimSpace(versionID); versionID != "" {
		input.VersionId = awssdk.String(versionID)
	}

	opCtx, cancel := m.operationContext(ctx)
	defer cancel()

	output, err := m.client.GetSecretValue(
		opCtx,
		input,
	)
	if err != nil {
		var notFoundErr *secretsTypes.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return "", fmt.Errorf("secret not found: %w", err)
		}

		return "", fmt.Errorf("get secret: %w", err)
	}

	if output == nil ||
		output.SecretString == nil ||
		*output.SecretString == "" {
		return "", errors.New("secret contains no string value")
	}

	return *output.SecretString, nil
}

func (m *AWSSecretManager) DeleteSecret(
	ctx context.Context,
	secretID string,
) error {
	if err := m.validate(); err != nil {
		return err
	}

	secretID = strings.TrimSpace(secretID)
	if secretID == "" {
		return errors.New("secret ID is required")
	}

	opCtx, cancel := m.operationContext(ctx)
	defer cancel()

	_, err := m.client.DeleteSecret(
		opCtx,
		&secretsmanager.DeleteSecretInput{
			SecretId: awssdk.String(secretID),

			// Keep AWS's recovery window.
			// Do NOT use ForceDeleteWithoutRecovery for customer secrets.
		},
	)
	if err != nil {
		var notFoundErr *secretsTypes.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			// Idempotent delete.
			return nil
		}

		return fmt.Errorf("delete secret: %w", err)
	}

	return nil
}

func (m *AWSSecretManager) validate() error {
	if m == nil || m.client == nil {
		return errors.New("Secrets Manager client is not initialized")
	}

	return nil
}

func (m *AWSSecretManager) operationContext(
	ctx context.Context,
) (context.Context, context.CancelFunc) {
	timeout := m.timeout
	if timeout <= 0 {
		timeout = defaultSecretOperationTimeout
	}

	return context.WithTimeout(ctx, timeout)
}

func secretReference(
	arn *string,
	versionID *string,
) (*SecretReference, error) {
	ref := &SecretReference{
		ARN:       strings.TrimSpace(awssdk.ToString(arn)),
		VersionID: strings.TrimSpace(awssdk.ToString(versionID)),
	}

	if ref.ARN == "" {
		return nil, errors.New("AWS returned an empty secret ARN")
	}

	if ref.VersionID == "" {
		return nil, errors.New("AWS returned an empty secret version ID")
	}

	return ref, nil
}

func (m *AWSSecretManager) RestoreSecret(
	ctx context.Context,
	secretID string,
) error {
	if err := m.validate(); err != nil {
		return err
	}

	secretID = strings.TrimSpace(secretID)
	if secretID == "" {
		return errors.New("secret ID is required")
	}

	opCtx, cancel := m.operationContext(ctx)
	defer cancel()

	_, err := m.client.RestoreSecret(
		opCtx,
		&secretsmanager.RestoreSecretInput{
			SecretId: awssdk.String(secretID),
		},
	)
	if err != nil {
		return fmt.Errorf("restore secret: %w", err)
	}

	return nil
}
