package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"elitegate/internal/model"

	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

var ErrProjectJWTConfigNotFound = errors.New("project JWT configuration not found")

type ProjectJWTConfigRepo struct {
	BaseRepo
}

func NewProjectJWTConfigRepo(
	db *sql.DB,
	logger zerolog.Logger,
) *ProjectJWTConfigRepo {
	return &ProjectJWTConfigRepo{
		BaseRepo: BaseRepo{
			db:     db,
			logger: logger,
		},
	}
}

const projectJWTConfigColumns = `
	project_id,
	enabled,
	algorithm,
	secret_arn,
	secret_version_id,
	config_version,
	issuer,
	audiences,
	subject_claim,
	role_claim,
	scopes_claim,
	clock_skew_seconds,
	created_by,
	updated_by,
	created_at,
	updated_at
`

type projectJWTConfigScanner interface {
	Scan(dest ...any) error
}

func scanProjectJWTConfig(
	scanner projectJWTConfigScanner,
) (*model.ProjectJWTConfig, error) {
	var cfg model.ProjectJWTConfig

	err := scanner.Scan(
		&cfg.ProjectID,
		&cfg.Enabled,
		&cfg.Algorithm,
		&cfg.SecretARN,
		&cfg.SecretVersionID,
		&cfg.ConfigVersion,
		&cfg.Issuer,
		pq.Array(&cfg.Audiences),
		&cfg.SubjectClaim,
		&cfg.RoleClaim,
		&cfg.ScopesClaim,
		&cfg.ClockSkewSeconds,
		&cfg.CreatedBy,
		&cfg.UpdatedBy,
		&cfg.CreatedAt,
		&cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if cfg.Audiences == nil {
		cfg.Audiences = []string{}
	}

	return &cfg, nil
}

// Get returns the JWT configuration belonging to the current tenant.
//
// Project identity comes exclusively from TenantContext. Callers cannot
// provide another project ID and accidentally access another tenant's config.
func (r *ProjectJWTConfigRepo) Get(
	ctx context.Context,
) (*model.ProjectJWTConfig, error) {
	var cfg *model.ProjectJWTConfig

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		query := fmt.Sprintf(`
			SELECT %s
			FROM project_jwt_configs
			WHERE project_id = $1
		`, projectJWTConfigColumns)

		cfg, err = scanProjectJWTConfig(
			tx.QueryRowContext(ctx, query, tc.ProjectID),
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProjectJWTConfigNotFound
		}
		if err != nil {
			return fmt.Errorf("query project JWT config: %w", err)
		}

		return nil
	})
	if errors.Is(err, ErrProjectJWTConfigNotFound) {
		return nil, ErrProjectJWTConfigNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project JWT config: %w", err)
	}

	return cfg, nil
}

// Upsert creates or updates the current project's JWT configuration.
//
// The project ID and user ID are always derived from TenantContext.
// config_version increments atomically on every update so gateways can
// efficiently detect configuration changes.
func (r *ProjectJWTConfigRepo) Upsert(
	ctx context.Context,
	cfg *model.ProjectJWTConfig,
) error {
	if cfg == nil {
		return errors.New("project JWT configuration is required")
	}

	return r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const query = `
			INSERT INTO project_jwt_configs (
				project_id,
				enabled,
				algorithm,
				secret_arn,
				secret_version_id,
				issuer,
				audiences,
				subject_claim,
				role_claim,
				scopes_claim,
				clock_skew_seconds,
				created_by,
				updated_by
			)
			VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10,
				$11, $12, $12
			)
			ON CONFLICT (project_id)
			DO UPDATE SET
				enabled            = EXCLUDED.enabled,
				algorithm          = EXCLUDED.algorithm,
				secret_arn         = EXCLUDED.secret_arn,
				secret_version_id  = EXCLUDED.secret_version_id,
				issuer             = EXCLUDED.issuer,
				audiences          = EXCLUDED.audiences,
				subject_claim      = EXCLUDED.subject_claim,
				role_claim         = EXCLUDED.role_claim,
				scopes_claim       = EXCLUDED.scopes_claim,
				clock_skew_seconds = EXCLUDED.clock_skew_seconds,
				updated_by         = EXCLUDED.updated_by,
				config_version     = project_jwt_configs.config_version + 1
			RETURNING
				config_version,
				created_by,
				updated_by,
				created_at,
				updated_at
		`

		cfg.ProjectID = tc.ProjectID.String()

		err = tx.QueryRowContext(
			ctx,
			query,
			tc.ProjectID,
			cfg.Enabled,
			cfg.Algorithm,
			cfg.SecretARN,
			cfg.SecretVersionID,
			cfg.Issuer,
			pq.Array(cfg.Audiences),
			cfg.SubjectClaim,
			cfg.RoleClaim,
			cfg.ScopesClaim,
			cfg.ClockSkewSeconds,
			tc.UserID,
		).Scan(
			&cfg.ConfigVersion,
			&cfg.CreatedBy,
			&cfg.UpdatedBy,
			&cfg.CreatedAt,
			&cfg.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("upsert project JWT config: %w", err)
		}

		return nil
	})
}

// Delete removes the JWT configuration belonging to the current project.
//
// AWS secret deletion will be handled by the service layer. The repository
// is intentionally responsible only for PostgreSQL state.
func (r *ProjectJWTConfigRepo) Delete(
	ctx context.Context,
) error {
	return r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		result, err := tx.ExecContext(
			ctx,
			`
				DELETE FROM project_jwt_configs
				WHERE project_id = $1
			`,
			tc.ProjectID,
		)
		if err != nil {
			return fmt.Errorf("delete project JWT config: %w", err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("get affected rows: %w", err)
		}

		if affected == 0 {
			return ErrProjectJWTConfigNotFound
		}

		return nil
	})
}
