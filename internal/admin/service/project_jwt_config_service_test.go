package service

import (
	"context"
	"errors"
	"testing"

	eliteaws "elitegate/internal/aws"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProjectJWTConfigRepo struct {
	cfg *model.ProjectJWTConfig

	getErr    error
	upsertErr error
	deleteErr error

	upsertCalls int
	deleteCalls int
}

func (f *fakeProjectJWTConfigRepo) Get(
	_ context.Context,
) (*model.ProjectJWTConfig, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}

	if f.cfg == nil {
		return nil, storage.ErrProjectJWTConfigNotFound
	}

	copyCfg := *f.cfg
	copyCfg.Audiences = append(
		[]string(nil),
		f.cfg.Audiences...,
	)

	return &copyCfg, nil
}

func (f *fakeProjectJWTConfigRepo) Upsert(
	_ context.Context,
	cfg *model.ProjectJWTConfig,
) error {
	f.upsertCalls++

	if f.upsertErr != nil {
		return f.upsertErr
	}

	copyCfg := *cfg

	if copyCfg.ConfigVersion == 0 {
		copyCfg.ConfigVersion = 1
	} else {
		copyCfg.ConfigVersion++
	}

	copyCfg.Audiences = append(
		[]string(nil),
		cfg.Audiences...,
	)

	// Simulate RETURNING config_version from PostgreSQL.
	cfg.ConfigVersion = copyCfg.ConfigVersion

	f.cfg = &copyCfg

	return nil
}

func (f *fakeProjectJWTConfigRepo) Delete(
	_ context.Context,
) error {
	f.deleteCalls++

	if f.deleteErr != nil {
		return f.deleteErr
	}

	f.cfg = nil

	return nil
}

type fakeJWTSecretManager struct {
	createRef *eliteaws.SecretReference
	updateRef *eliteaws.SecretReference

	createErr  error
	updateErr  error
	deleteErr  error
	restoreErr error

	createCalls  int
	updateCalls  int
	deleteCalls  int
	restoreCalls int
	getCalls     int

	createdName  string
	createdValue string
	updatedID    string
	updatedValue string
	deletedID    string
	restoredID   string
}

func (f *fakeJWTSecretManager) CreateSecret(
	_ context.Context,
	name string,
	value string,
) (*eliteaws.SecretReference, error) {
	f.createCalls++
	f.createdName = name
	f.createdValue = value

	if f.createErr != nil {
		return nil, f.createErr
	}

	return f.createRef, nil
}

func (f *fakeJWTSecretManager) UpdateSecret(
	_ context.Context,
	secretID string,
	value string,
) (*eliteaws.SecretReference, error) {
	f.updateCalls++
	f.updatedID = secretID
	f.updatedValue = value

	if f.updateErr != nil {
		return nil, f.updateErr
	}

	return f.updateRef, nil
}

func (f *fakeJWTSecretManager) GetSecret(
	_ context.Context,
	_ string,
	_ string,
) (string, error) {
	f.getCalls++
	return "", nil
}

func (f *fakeJWTSecretManager) DeleteSecret(
	_ context.Context,
	secretID string,
) error {
	f.deleteCalls++
	f.deletedID = secretID

	return f.deleteErr
}

func (f *fakeJWTSecretManager) RestoreSecret(
	_ context.Context,
	secretID string,
) error {
	f.restoreCalls++
	f.restoredID = secretID

	return f.restoreErr
}

func jwtServiceTenantContext() context.Context {
	return storage.WithTenantContext(
		context.Background(),
		storage.TenantContext{
			ProjectID: uuid.MustParse(
				"11111111-1111-1111-1111-111111111111",
			),
			UserID: uuid.MustParse(
				"22222222-2222-2222-2222-222222222222",
			),
			UserRole: "owner",
		},
	)
}

func validJWTConfigInput() ProjectJWTConfigInput {
	clockSkew := 30

	return ProjectJWTConfigInput{
		Enabled:   true,
		Algorithm: model.JWTAlgorithmHS256,

		Secret: "12345678901234567890123456789012",

		Audiences: []string{
			"yumzy-api",
		},

		SubjectClaim: "sub",
		RoleClaim:    "role",
		ScopesClaim:  "scope",

		ClockSkewSeconds: &clockSkew,
	}
}

func existingJWTConfig() *model.ProjectJWTConfig {
	return &model.ProjectJWTConfig{
		ProjectID: "11111111-1111-1111-1111-111111111111",

		Enabled:   true,
		Algorithm: model.JWTAlgorithmHS256,

		SecretARN: "arn:aws:secretsmanager:ap-south-1:123456789012:secret:test",

		SecretVersionID: "version-1",

		ConfigVersion: 1,

		Audiences: []string{
			"yumzy-api",
		},

		SubjectClaim: "sub",
		RoleClaim:    "role",
		ScopesClaim:  "scope",

		ClockSkewSeconds: 30,
	}
}

func TestProjectJWTConfigService_Create(
	t *testing.T,
) {
	repo := &fakeProjectJWTConfigRepo{}

	secrets := &fakeJWTSecretManager{
		createRef: &eliteaws.SecretReference{
			ARN:       "arn:aws:secretsmanager:ap-south-1:123456789012:secret:new",
			VersionID: "version-1",
		},
	}

	svc := NewProjectJWTConfigService(
		repo,
		secrets,
		"production",
		zerolog.Nop(),
	)

	cfg, err := svc.Configure(
		jwtServiceTenantContext(),
		validJWTConfigInput(),
	)

	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 1, secrets.createCalls)
	assert.Equal(t, 1, repo.upsertCalls)

	assert.Equal(
		t,
		"elitegate/production/projects/11111111-1111-1111-1111-111111111111/jwt/hs256",
		secrets.createdName,
	)

	assert.Equal(
		t,
		"version-1",
		cfg.SecretVersionID,
	)

	assert.Equal(t, int64(1), cfg.ConfigVersion)

	// Secret must not become a model/database field.
	assert.NotEqual(
		t,
		secrets.createdValue,
		cfg.SecretARN,
	)
}

func TestProjectJWTConfigService_CreateDBFailureCleansAWS(
	t *testing.T,
) {
	dbErr := errors.New("database unavailable")

	repo := &fakeProjectJWTConfigRepo{
		upsertErr: dbErr,
	}

	secrets := &fakeJWTSecretManager{
		createRef: &eliteaws.SecretReference{
			ARN:       "arn:aws:secretsmanager:ap-south-1:123456789012:secret:new",
			VersionID: "version-1",
		},
	}

	svc := NewProjectJWTConfigService(
		repo,
		secrets,
		"production",
		zerolog.Nop(),
	)

	_, err := svc.Configure(
		jwtServiceTenantContext(),
		validJWTConfigInput(),
	)

	require.Error(t, err)

	assert.Equal(t, 1, secrets.createCalls)
	assert.Equal(t, 1, secrets.deleteCalls)

	assert.Equal(
		t,
		secrets.createRef.ARN,
		secrets.deletedID,
	)
}

func TestProjectJWTConfigService_NoOpUpdate(
	t *testing.T,
) {
	current := existingJWTConfig()

	repo := &fakeProjectJWTConfigRepo{
		cfg: current,
	}

	secrets := &fakeJWTSecretManager{}

	svc := NewProjectJWTConfigService(
		repo,
		secrets,
		"production",
		zerolog.Nop(),
	)

	input := validJWTConfigInput()

	// Blank means keep the current secret.
	input.Secret = ""

	result, err := svc.Configure(
		jwtServiceTenantContext(),
		input,
	)

	require.NoError(t, err)

	assert.Equal(t, 0, repo.upsertCalls)
	assert.Equal(t, 0, secrets.createCalls)
	assert.Equal(t, 0, secrets.updateCalls)

	assert.Equal(
		t,
		current.ConfigVersion,
		result.ConfigVersion,
	)
}

func TestProjectJWTConfigService_RotateSecret(
	t *testing.T,
) {
	repo := &fakeProjectJWTConfigRepo{
		cfg: existingJWTConfig(),
	}

	secrets := &fakeJWTSecretManager{
		updateRef: &eliteaws.SecretReference{
			ARN:       "arn:aws:secretsmanager:ap-south-1:123456789012:secret:test",
			VersionID: "version-2",
		},
	}

	svc := NewProjectJWTConfigService(
		repo,
		secrets,
		"production",
		zerolog.Nop(),
	)

	input := validJWTConfigInput()

	input.Secret =
		"abcdefghijklmnopqrstuvwxyz123456"

	cfg, err := svc.Configure(
		jwtServiceTenantContext(),
		input,
	)

	require.NoError(t, err)

	assert.Equal(t, 1, secrets.updateCalls)
	assert.Equal(t, 1, repo.upsertCalls)

	assert.Equal(
		t,
		"version-2",
		cfg.SecretVersionID,
	)

	assert.Equal(
		t,
		int64(2),
		cfg.ConfigVersion,
	)
}

func TestProjectJWTConfigService_Delete(
	t *testing.T,
) {
	current := existingJWTConfig()

	repo := &fakeProjectJWTConfigRepo{
		cfg: current,
	}

	secrets := &fakeJWTSecretManager{}

	svc := NewProjectJWTConfigService(
		repo,
		secrets,
		"production",
		zerolog.Nop(),
	)

	err := svc.Delete(
		jwtServiceTenantContext(),
	)

	require.NoError(t, err)

	assert.Equal(t, 1, repo.deleteCalls)
	assert.Equal(t, 1, secrets.deleteCalls)

	assert.Equal(
		t,
		current.SecretARN,
		secrets.deletedID,
	)
}

func TestProjectJWTConfigService_AWSDeleteFailureRestoresDB(
	t *testing.T,
) {
	awsErr := errors.New(
		"Secrets Manager unavailable",
	)

	current := existingJWTConfig()

	repo := &fakeProjectJWTConfigRepo{
		cfg: current,
	}

	secrets := &fakeJWTSecretManager{
		deleteErr: awsErr,
	}

	svc := NewProjectJWTConfigService(
		repo,
		secrets,
		"production",
		zerolog.Nop(),
	)

	err := svc.Delete(
		jwtServiceTenantContext(),
	)

	require.Error(t, err)

	assert.Equal(t, 1, repo.deleteCalls)
	assert.Equal(t, 1, secrets.deleteCalls)

	// Compensation restored PostgreSQL metadata.
	assert.Equal(t, 1, repo.upsertCalls)
	require.NotNil(t, repo.cfg)

	assert.Equal(
		t,
		current.SecretARN,
		repo.cfg.SecretARN,
	)
}

func TestProjectJWTConfigService_Create_SecretAlreadyExistsRestoresAndUpdate(
	t *testing.T,
) {
	repo := &fakeProjectJWTConfigRepo{}

	secrets := &fakeJWTSecretManager{
		createErr: eliteaws.ErrSecretAlreadyExists,
		updateRef: &eliteaws.SecretReference{
			ARN:       "arn:aws:secretsmanager:ap-south-1:123456789012:secret:restored",
			VersionID: "version-restored-1",
		},
	}

	svc := NewProjectJWTConfigService(
		repo,
		secrets,
		"production",
		zerolog.Nop(),
	)

	cfg, err := svc.Configure(
		jwtServiceTenantContext(),
		validJWTConfigInput(),
	)

	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 1, secrets.createCalls)
	assert.Equal(t, 1, secrets.restoreCalls)
	assert.Equal(t, 1, secrets.updateCalls)
	assert.Equal(t, 1, repo.upsertCalls)

	expectedSecretName := "elitegate/production/projects/11111111-1111-1111-1111-111111111111/jwt/hs256"
	assert.Equal(t, expectedSecretName, secrets.restoredID)
	assert.Equal(t, expectedSecretName, secrets.updatedID)

	assert.Equal(t, "version-restored-1", cfg.SecretVersionID)
	assert.Equal(t, "arn:aws:secretsmanager:ap-south-1:123456789012:secret:restored", cfg.SecretARN)
}
