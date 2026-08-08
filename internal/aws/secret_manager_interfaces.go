package aws

import (
	"context"
	"errors"
)

var ErrSecretAlreadyExists = errors.New("secret already exists")

// SecretReference identifies a specific version of a secret stored in AWS
// Secrets Manager.
//
// SecretValue is deliberately not included here to reduce the chance of
// accidentally logging or serializing secret material.
type SecretReference struct {
	ARN       string
	VersionID string
}

// SecretManager defines the secret operations required by EliteGate.
//
// The interface is intentionally small so callers depend on EliteGate's
// abstraction rather than directly on the AWS SDK.
type SecretManager interface {
	CreateSecret(
		ctx context.Context,
		name string,
		secretValue string,
	) (*SecretReference, error)

	UpdateSecret(
		ctx context.Context,
		secretID string,
		secretValue string,
	) (*SecretReference, error)

	GetSecret(
		ctx context.Context,
		secretID string,
		versionID string,
	) (string, error)

	DeleteSecret(
		ctx context.Context,
		secretID string,
	) error

	RestoreSecret(
		ctx context.Context,
		secretID string,
	) error
}
