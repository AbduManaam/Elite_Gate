package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	eliteaws "elitegate/internal/aws"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

const (
	defaultJWTClockSkewSeconds = 30
	minimumHS256SecretBytes    = 32
	maximumJWTSecretBytes      = 4096
)

var (
	ErrJWTSecretRequired = errors.New(
		"JWT secret is required when configuring JWT authentication",
	)

	ErrJWTSecretTooShort = errors.New(
		"HS256 JWT secret must be at least 32 bytes",
	)

	ErrJWTSecretTooLarge = errors.New(
		"JWT secret is too large",
	)

	ErrUnsupportedJWTAlgorithm = errors.New(
		"unsupported JWT algorithm",
	)

	ErrInvalidJWTConfig = errors.New(
		"invalid JWT configuration",
	)
)

// ProjectJWTConfigRepository contains only the persistence operations
// required by the service.
//
// Using an interface keeps the service independently testable.
type ProjectJWTConfigRepository interface {
	Get(
		ctx context.Context,
	) (*model.ProjectJWTConfig, error)

	Upsert(
		ctx context.Context,
		cfg *model.ProjectJWTConfig,
	) error

	Delete(
		ctx context.Context,
	) error
}

// ProjectJWTConfigInput contains configuration supplied by the Admin API.
//
// Secret is write-only. It is never included in a response object and is
// never stored in PostgreSQL.
type ProjectJWTConfigInput struct {
	Enabled bool

	Algorithm string

	Secret string

	Issuer *string

	Audiences []string

	SubjectClaim string
	RoleClaim    string
	ScopesClaim  string

	// Pointer allows zero to be intentionally configured.
	ClockSkewSeconds *int
}

// ProjectJWTConfigService coordinates PostgreSQL metadata and AWS
// Secrets Manager.
//
// Customer secret material exists only:
//   - in the incoming request memory;
//   - while being sent to AWS Secrets Manager;
//   - later in gateway memory when the gateway loads it.
//
// It is never persisted in PostgreSQL.
type ProjectJWTConfigService struct {
	repo        ProjectJWTConfigRepository
	secrets     eliteaws.SecretManager
	environment string
	logger      zerolog.Logger
}

func NewProjectJWTConfigService(
	repo ProjectJWTConfigRepository,
	secrets eliteaws.SecretManager,
	environment string,
	logger zerolog.Logger,
) *ProjectJWTConfigService {
	return &ProjectJWTConfigService{
		repo:        repo,
		secrets:     secrets,
		environment: normalizeEnvironment(environment),
		logger: logger.With().
			Str("service", "project_jwt_config").
			Logger(),
	}
}

// Get returns the current project's JWT configuration.
//
// SecretARN and SecretVersionID remain internal because the model marks
// those fields json:"-".
func (s *ProjectJWTConfigService) Get(
	ctx context.Context,
) (*model.ProjectJWTConfig, error) {
	cfg, err := s.repo.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("get project JWT configuration: %w", err)
	}

	return cfg, nil
}

// Configure creates or updates JWT authentication for the current project.
//
// Creation:
//
//	AWS CreateSecret -> PostgreSQL Upsert
//
// If PostgreSQL creation fails, the newly-created AWS secret is scheduled
// for deletion as compensation.
//
// Update:
//
//	settings only -> PostgreSQL
//	secret changed -> AWS new secret version -> PostgreSQL stores new version
//
// PostgreSQL stores the exact AWS VersionID. Therefore if the database
// update fails after rotating the secret, existing gateways continue using
// the previously referenced version rather than silently switching.
func (s *ProjectJWTConfigService) Configure(
	ctx context.Context,
	input ProjectJWTConfigInput,
) (*model.ProjectJWTConfig, error) {
	if s.repo == nil {
		return nil, errors.New("JWT config repository is not initialized")
	}

	if s.secrets == nil {
		return nil, errors.New("Secrets Manager is not initialized")
	}

	normalized, err := normalizeJWTConfigInput(input)
	if err != nil {
		return nil, err
	}

	current, err := s.repo.Get(ctx)

	switch {
	case err == nil:
		return s.update(ctx, current, normalized)

	case errors.Is(err, storage.ErrProjectJWTConfigNotFound):
		return s.create(ctx, normalized)

	default:
		return nil, fmt.Errorf(
			"load existing project JWT configuration: %w",
			err,
		)
	}
}

func (s *ProjectJWTConfigService) create(
	ctx context.Context,
	input ProjectJWTConfigInput,
) (*model.ProjectJWTConfig, error) {
	if input.Secret == "" {
		return nil, ErrJWTSecretRequired
	}

	tc, err := storage.TenantFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("read tenant context: %w", err)
	}

	secretName := projectJWTSecretName(
		s.environment,
		tc.ProjectID.String(),
	)

	ref, err := s.secrets.CreateSecret(
		ctx,
		secretName,
		input.Secret,
	)

	if errors.Is(err, eliteaws.ErrSecretAlreadyExists) {
		// A previous interrupted operation may have left this
		// deterministic project secret behind.
		//
		// RestoreSecret is best-effort. If the secret wasn't scheduled
		// for deletion, UpdateSecret below will simply use the existing
		// active secret.
		_ = s.secrets.RestoreSecret(
			context.WithoutCancel(ctx),
			secretName,
		)

		ref, err = s.secrets.UpdateSecret(
			ctx,
			secretName,
			input.Secret,
		)
	}

	if err != nil {
		return nil, fmt.Errorf(
			"create project JWT secret: %w",
			err,
		)
	}

	cfg := configFromInput(input)

	cfg.SecretARN = ref.ARN
	cfg.SecretVersionID = ref.VersionID

	if err := s.repo.Upsert(ctx, cfg); err != nil {
		// Best-effort compensation.
		//
		// Do not return a cleanup error instead of the real database error.
		// Also never log the secret value.
		if cleanupErr := s.secrets.DeleteSecret(
			context.WithoutCancel(ctx),
			ref.ARN,
		); cleanupErr != nil {
			s.logger.Error().
				Err(cleanupErr).
				Str("project_id", tc.ProjectID.String()).
				Msg("failed to clean up JWT secret after database failure")
		}

		return nil, fmt.Errorf(
			"persist project JWT configuration: %w",
			err,
		)
	}

	s.logger.Info().
		Str("project_id", tc.ProjectID.String()).
		Msg("project JWT authentication configured")

	return cfg, nil
}

func (s *ProjectJWTConfigService) update(
	ctx context.Context,
	current *model.ProjectJWTConfig,
	input ProjectJWTConfigInput,
) (*model.ProjectJWTConfig, error) {
	if current == nil {
		return nil, errors.New("existing JWT configuration is nil")
	}

	next := configFromInput(input)

	next.SecretARN = current.SecretARN
	next.SecretVersionID = current.SecretVersionID
	next.ConfigVersion = current.ConfigVersion
	next.ProjectID = current.ProjectID
	next.CreatedAt = current.CreatedAt
	next.CreatedBy = current.CreatedBy

	secretChanged := input.Secret != ""

	// Avoid unnecessary PostgreSQL writes, config_version increments
	// and gateway verifier reloads when nothing actually changed.
	if !secretChanged && jwtConfigEquivalent(current, next) {
		return current, nil
	}

	if secretChanged {
		ref, err := s.secrets.UpdateSecret(
			ctx,
			current.SecretARN,
			input.Secret,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"rotate project JWT secret: %w",
				err,
			)
		}

		next.SecretARN = ref.ARN
		next.SecretVersionID = ref.VersionID
	}

	if err := s.repo.Upsert(ctx, next); err != nil {
		// Important:
		//
		// If PutSecretValue succeeded but PostgreSQL failed, AWS may contain
		// an unused newer version. We intentionally do NOT switch gateways
		// to it. PostgreSQL still references the previous VersionID, so the
		// active runtime remains consistent.
		return nil, fmt.Errorf(
			"persist updated project JWT configuration: %w",
			err,
		)
	}

	tc, tenantErr := storage.TenantFromContext(ctx)
	if tenantErr == nil {
		s.logger.Info().
			Str("project_id", tc.ProjectID.String()).
			Bool("secret_rotated", secretChanged).
			Int64("config_version", next.ConfigVersion).
			Msg("project JWT authentication updated")
	}

	return next, nil
}

// Delete removes JWT authentication from the project.
//
// AWS deletion is scheduled first. If PostgreSQL deletion fails, the AWS
// secret is restored as compensation.
func (s *ProjectJWTConfigService) Delete(
	ctx context.Context,
) error {
	if s.repo == nil {
		return errors.New("JWT config repository is not initialized")
	}

	if s.secrets == nil {
		return errors.New("Secrets Manager is not initialized")
	}

	current, err := s.repo.Get(ctx)
	if err != nil {
		return fmt.Errorf(
			"load project JWT configuration for deletion: %w",
			err,
		)
	}

	// Remove the active application reference first.
	//
	// This is safer than deleting the AWS secret first because a secret
	// scheduled for deletion cannot be read by gateways until restored.
	if err := s.repo.Delete(ctx); err != nil {
		return fmt.Errorf(
			"delete project JWT configuration: %w",
			err,
		)
	}

	// Then schedule the actual secret for deletion.
	if err := s.secrets.DeleteSecret(
		ctx,
		current.SecretARN,
	); err != nil {

		// Best-effort compensation: put the DB configuration back because
		// AWS deletion did not succeed.
		if restoreErr := s.repo.Upsert(
			context.WithoutCancel(ctx),
			current,
		); restoreErr != nil {
			s.logger.Error().
				Err(restoreErr).
				Msg("failed to restore JWT configuration after AWS deletion failure")
		}

		return fmt.Errorf(
			"schedule project JWT secret deletion: %w",
			err,
		)
	}

	if tc, err := storage.TenantFromContext(ctx); err == nil {
		s.logger.Info().
			Str("project_id", tc.ProjectID.String()).
			Msg("project JWT authentication removed")
	}

	return nil
}

func configFromInput(
	input ProjectJWTConfigInput,
) *model.ProjectJWTConfig {
	clockSkew := defaultJWTClockSkewSeconds

	if input.ClockSkewSeconds != nil {
		clockSkew = *input.ClockSkewSeconds
	}

	return &model.ProjectJWTConfig{
		Enabled:          input.Enabled,
		Algorithm:        input.Algorithm,
		Issuer:           input.Issuer,
		Audiences:        append([]string(nil), input.Audiences...),
		SubjectClaim:     input.SubjectClaim,
		RoleClaim:        input.RoleClaim,
		ScopesClaim:      input.ScopesClaim,
		ClockSkewSeconds: clockSkew,
	}
}

func normalizeJWTConfigInput(
	input ProjectJWTConfigInput,
) (ProjectJWTConfigInput, error) {
	input.Algorithm = strings.ToUpper(
		strings.TrimSpace(input.Algorithm),
	)

	if input.Algorithm == "" {
		input.Algorithm = model.JWTAlgorithmHS256
	}

	if input.Algorithm != model.JWTAlgorithmHS256 {
		return ProjectJWTConfigInput{}, ErrUnsupportedJWTAlgorithm
	}

	if input.Secret != "" {
		if strings.TrimSpace(input.Secret) == "" {
			return ProjectJWTConfigInput{}, ErrJWTSecretRequired
		}

		if len([]byte(input.Secret)) < minimumHS256SecretBytes {
			return ProjectJWTConfigInput{}, ErrJWTSecretTooShort
		}

		if len([]byte(input.Secret)) > maximumJWTSecretBytes {
			return ProjectJWTConfigInput{}, ErrJWTSecretTooLarge
		}
	}

	input.SubjectClaim = defaultClaimName(
		input.SubjectClaim,
		"sub",
	)

	input.RoleClaim = defaultClaimName(
		input.RoleClaim,
		"role",
	)

	input.ScopesClaim = defaultClaimName(
		input.ScopesClaim,
		"scope",
	)

	if err := validateClaimName(input.SubjectClaim); err != nil {
		return ProjectJWTConfigInput{}, fmt.Errorf(
			"subject claim: %w",
			err,
		)
	}

	if err := validateClaimName(input.RoleClaim); err != nil {
		return ProjectJWTConfigInput{}, fmt.Errorf(
			"role claim: %w",
			err,
		)
	}

	if err := validateClaimName(input.ScopesClaim); err != nil {
		return ProjectJWTConfigInput{}, fmt.Errorf(
			"scopes claim: %w",
			err,
		)
	}

	if input.Issuer != nil {
		issuer := strings.TrimSpace(*input.Issuer)

		if issuer == "" {
			input.Issuer = nil
		} else {
			if len(issuer) > 512 {
				return ProjectJWTConfigInput{}, fmt.Errorf(
					"%w: issuer is too long",
					ErrInvalidJWTConfig,
				)
			}

			input.Issuer = &issuer
		}
	}

	audiences, err := normalizeAudiences(input.Audiences)
	if err != nil {
		return ProjectJWTConfigInput{}, err
	}

	input.Audiences = audiences

	if input.ClockSkewSeconds != nil {
		if *input.ClockSkewSeconds < 0 ||
			*input.ClockSkewSeconds > 300 {
			return ProjectJWTConfigInput{}, fmt.Errorf(
				"%w: clock skew must be between 0 and 300 seconds",
				ErrInvalidJWTConfig,
			)
		}
	}

	return input, nil
}

func normalizeAudiences(
	values []string,
) ([]string, error) {
	if len(values) > 20 {
		return nil, fmt.Errorf(
			"%w: too many audiences",
			ErrInvalidJWTConfig,
		)
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)

		if value == "" {
			continue
		}

		if len(value) > 512 {
			return nil, fmt.Errorf(
				"%w: audience is too long",
				ErrInvalidJWTConfig,
			)
		}

		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}
		result = append(result, value)
	}

	// Audience order has no authentication meaning.
	// Canonical ordering prevents unnecessary config updates.
	slices.Sort(result)

	return result, nil
}

func jwtConfigEquivalent(
	current *model.ProjectJWTConfig,
	next *model.ProjectJWTConfig,
) bool {
	if current == nil || next == nil {
		return false
	}

	return current.Enabled == next.Enabled &&
		current.Algorithm == next.Algorithm &&
		optionalStringEqual(current.Issuer, next.Issuer) &&
		slices.Equal(current.Audiences, next.Audiences) &&
		current.SubjectClaim == next.SubjectClaim &&
		current.RoleClaim == next.RoleClaim &&
		current.ScopesClaim == next.ScopesClaim &&
		current.ClockSkewSeconds == next.ClockSkewSeconds
}

func optionalStringEqual(
	a *string,
	b *string,
) bool {
	switch {
	case a == nil && b == nil:
		return true

	case a == nil || b == nil:
		return false

	default:
		return *a == *b
	}
}

func defaultClaimName(
	value string,
	fallback string,
) string {
	value = strings.TrimSpace(value)

	if value == "" {
		return fallback
	}

	return value
}

func validateClaimName(
	value string,
) error {
	if value == "" {
		return fmt.Errorf(
			"%w: claim name cannot be empty",
			ErrInvalidJWTConfig,
		)
	}

	if len(value) > 128 {
		return fmt.Errorf(
			"%w: claim name is too long",
			ErrInvalidJWTConfig,
		)
	}

	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf(
				"%w: claim name contains control characters",
				ErrInvalidJWTConfig,
			)
		}
	}

	return nil
}

func projectJWTSecretName(
	environment string,
	projectID string,
) string {
	return fmt.Sprintf(
		"elitegate/%s/projects/%s/jwt/hs256",
		normalizeEnvironment(environment),
		projectID,
	)
}

func normalizeEnvironment(
	value string,
) string {
	value = strings.ToLower(strings.TrimSpace(value))

	if value == "" {
		return "development"
	}

	var builder strings.Builder

	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)

		case r >= '0' && r <= '9':
			builder.WriteRune(r)

		case r == '-':
			builder.WriteRune(r)

		default:
			builder.WriteRune('-')
		}
	}

	result := strings.Trim(
		builder.String(),
		"-",
	)

	if result == "" {
		return "development"
	}

	return result
}
