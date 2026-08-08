package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"elitegate/internal/auth"
	"elitegate/internal/model"

	"github.com/rs/zerolog"
)

// ProjectJWTSecretReader contains only the Secrets Manager operation the
// gateway runtime actually needs.
//
// This keeps the runtime independent from create/update/delete operations.
type ProjectJWTSecretReader interface {
	GetSecret(
		ctx context.Context,
		secretID string,
		versionID string,
	) (string, error)
}

// projectJWTState is immutable once stored.
//
// Requests read this structure through atomic.Pointer, so there is no
// per-request mutex.
type projectJWTState struct {
	enabled bool

	configVersion int64

	secretARN       string
	secretVersionID string

	verifier *auth.ProjectJWTVerifier
}

type ProjectJWTManager struct {
	secrets ProjectJWTSecretReader
	logger  zerolog.Logger

	// Configuration reloads are serialized.
	applyMu sync.Mutex

	// Request path uses only this atomic pointer.
	state atomic.Pointer[projectJWTState]
}

func NewProjectJWTManager(
	secrets ProjectJWTSecretReader,
	logger zerolog.Logger,
) *ProjectJWTManager {
	return &ProjectJWTManager{
		secrets: secrets,
		logger: logger.With().
			Str(
				"component",
				"project_jwt_manager",
			).
			Logger(),
	}
}

// Apply installs a new JWT configuration.
//
// AWS is called only when the underlying secret ARN/version changes.
func (m *ProjectJWTManager) Apply(
	ctx context.Context,
	cfg *model.ProjectJWTConfigSync,
) error {
	m.applyMu.Lock()
	defer m.applyMu.Unlock()

	current := m.state.Load()

	// No JWT config or explicitly disabled.
	if cfg == nil || !cfg.Enabled {
		version := int64(0)

		if cfg != nil {
			version = cfg.ConfigVersion
		}

		m.state.Store(
			&projectJWTState{
				enabled:       false,
				configVersion: version,
			},
		)

		return nil
	}

	if cfg.SecretARN == "" {
		return errors.New(
			"enabled project JWT configuration has no secret ARN",
		)
	}

	if cfg.SecretVersionID == "" {
		return errors.New(
			"enabled project JWT configuration has no secret version",
		)
	}

	// Exact same configuration already active.
	if current != nil &&
		current.enabled &&
		current.configVersion == cfg.ConfigVersion &&
		current.secretARN == cfg.SecretARN &&
		current.secretVersionID == cfg.SecretVersionID &&
		current.verifier != nil {

		return nil
	}

	verifierConfig :=
		projectJWTVerifierConfig(cfg)

	// Non-secret settings changed but the AWS secret did not.
	//
	// Example:
	// audience v1 -> audience v2
	//
	// Rebuild locally from the existing verifier.
	// NO AWS request.
	if current != nil &&
		current.enabled &&
		current.verifier != nil &&
		current.secretARN == cfg.SecretARN &&
		current.secretVersionID == cfg.SecretVersionID {

		verifier, err :=
			current.verifier.Reconfigure(
				verifierConfig,
			)

		if err != nil {
			return fmt.Errorf(
				"reconfigure project JWT verifier: %w",
				err,
			)
		}

		m.state.Store(
			&projectJWTState{
				enabled: true,

				configVersion: cfg.ConfigVersion,

				secretARN: cfg.SecretARN,

				secretVersionID: cfg.SecretVersionID,

				verifier: verifier,
			},
		)

		m.logger.Info().
			Int64(
				"config_version",
				cfg.ConfigVersion,
			).
			Msg(
				"project JWT verifier reconfigured without secret reload",
			)

		return nil
	}

	if m.secrets == nil {
		return errors.New(
			"project JWT Secrets Manager reader is not initialized",
		)
	}

	// First load or actual secret rotation.
	secret, err := m.secrets.GetSecret(
		ctx,
		cfg.SecretARN,
		cfg.SecretVersionID,
	)

	if err != nil {
		return fmt.Errorf(
			"load project JWT verification secret: %w",
			err,
		)
	}

	verifier, err :=
		auth.NewProjectJWTVerifier(
			secret,
			verifierConfig,
		)

	// Best effort: release our temporary reference.
	// The verifier keeps its own copy.
	secret = ""

	if err != nil {
		return fmt.Errorf(
			"build project JWT verifier: %w",
			err,
		)
	}

	// Only publish the state after every operation succeeded.
	//
	// Failed refresh therefore leaves the previous verifier active.
	m.state.Store(
		&projectJWTState{
			enabled: true,

			configVersion: cfg.ConfigVersion,

			secretARN: cfg.SecretARN,

			secretVersionID: cfg.SecretVersionID,

			verifier: verifier,
		},
	)

	m.logger.Info().
		Int64(
			"config_version",
			cfg.ConfigVersion,
		).
		Str(
			"algorithm",
			cfg.Algorithm,
		).
		Msg(
			"project JWT verifier activated",
		)

	return nil
}

// ValidateIdentity is the hot request path.
//
// No lock.
// No database.
// No Redis.
// No Admin API.
// No AWS.
func (m *ProjectJWTManager) ValidateIdentity(
	token string,
) (*auth.Identity, error) {
	state := m.state.Load()

	if state == nil ||
		!state.enabled ||
		state.verifier == nil {

		return nil,
			auth.ErrProjectJWTNotConfigured
	}

	return state.verifier.ValidateIdentity(
		token,
	)
}

func projectJWTVerifierConfig(
	cfg *model.ProjectJWTConfigSync,
) auth.ProjectJWTVerifierConfig {
	return auth.ProjectJWTVerifierConfig{
		Algorithm: cfg.Algorithm,

		Issuer: cfg.Issuer,

		Audiences: append(
			[]string(nil),
			cfg.Audiences...,
		),

		SubjectClaim: cfg.SubjectClaim,

		RoleClaim: cfg.RoleClaim,

		ScopesClaim: cfg.ScopesClaim,

		ClockSkew: time.Duration(
			cfg.ClockSkewSeconds,
		) * time.Second,
	}
}
