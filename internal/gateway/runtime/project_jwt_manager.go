package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"elitegate/internal/auth"
	eliteaws "elitegate/internal/aws"
	"elitegate/internal/model"

	"github.com/rs/zerolog"
)

type ProjectJWTManager struct {
	secrets eliteaws.SecretManager
	logger  zerolog.Logger

	applyMu sync.Mutex

	verifier atomic.Pointer[auth.ProjectJWTVerifier]

	configVersion   int64
	secretVersionID string
	enabled         bool
}

func NewProjectJWTManager(
	secrets eliteaws.SecretManager,
	logger zerolog.Logger,
) *ProjectJWTManager {
	return &ProjectJWTManager{
		secrets: secrets,
		logger: logger.With().
			Str("component", "project_jwt_manager").
			Logger(),
	}
}

func (m *ProjectJWTManager) Apply(
	ctx context.Context,
	cfg *model.ProjectJWTConfigSync,
) error {
	m.applyMu.Lock()
	defer m.applyMu.Unlock()

	if cfg == nil || !cfg.Enabled {
		m.verifier.Store(nil)
		m.enabled = false

		if cfg != nil {
			m.configVersion = cfg.ConfigVersion
		} else {
			m.configVersion = 0
		}

		m.secretVersionID = ""

		return nil
	}

	if m.enabled &&
		m.configVersion == cfg.ConfigVersion &&
		m.secretVersionID == cfg.SecretVersionID {

		// Most reloads come through here.
		// No AWS call.
		return nil
	}

	if m.secrets == nil {
		return errors.New(
			"Secrets Manager is not initialized",
		)
	}

	if cfg.SecretARN == "" ||
		cfg.SecretVersionID == "" {
		return errors.New(
			"enabled JWT configuration has no secret reference",
		)
	}

	secret, err := m.secrets.GetSecret(
		ctx,
		cfg.SecretARN,
		cfg.SecretVersionID,
	)
	if err != nil {
		return fmt.Errorf(
			"load project JWT secret: %w",
			err,
		)
	}

	verifier, err := auth.NewProjectJWTVerifier(
		secret,
		auth.ProjectJWTVerifierConfig{
			Algorithm:    cfg.Algorithm,
			Issuer:       cfg.Issuer,
			Audiences:    cfg.Audiences,
			SubjectClaim: cfg.SubjectClaim,
			RoleClaim:    cfg.RoleClaim,
			ScopesClaim:  cfg.ScopesClaim,
			ClockSkew: time.Duration(
				cfg.ClockSkewSeconds,
			) * time.Second,
		},
	)

	// Do not retain the temporary string any longer than needed.
	secret = ""

	if err != nil {
		return fmt.Errorf(
			"build project JWT verifier: %w",
			err,
		)
	}

	m.verifier.Store(verifier)

	m.configVersion = cfg.ConfigVersion
	m.secretVersionID = cfg.SecretVersionID
	m.enabled = true

	m.logger.Info().
		Int64(
			"config_version",
			cfg.ConfigVersion,
		).
		Msg("project JWT verifier reloaded")

	return nil
}

func (m *ProjectJWTManager) Validate(
	token string,
) (*auth.Identity, error) {
	verifier := m.verifier.Load()

	if verifier == nil {
		return nil, auth.ErrProjectJWTNotConfigured
	}

	// Hot request path:
	// atomic pointer read + local HMAC verification.
	// No Redis, PostgreSQL or AWS request.
	return verifier.Validate(token)
}
