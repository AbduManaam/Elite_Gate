package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"elitegate/internal/model"

	"github.com/rs/zerolog"
)

type UpstreamRepo struct {
	BaseRepo
}

func NewUpstreamRepo(db *sql.DB, logger zerolog.Logger) *UpstreamRepo {
	return &UpstreamRepo{BaseRepo{db: db, logger: logger}}
}

var (
	ErrUpstreamNotFound     = errors.New("upstream not found")
	ErrUpstreamNameConflict = errors.New("upstream name already exists")
)

func (r *UpstreamRepo) Create(ctx context.Context, u *model.Upstream) error {
	return r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		u.ProjectID = tc.ProjectID.String()

		const q = `
			INSERT INTO upstreams (
				project_id,
				name,
				target_url,
				protocol,
				health_path,
				lb_strategy,
				enabled
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, created_at, updated_at
		`

		err = tx.QueryRowContext(
			ctx,
			q,
			tc.ProjectID,
			u.Name,
			u.TargetURL,
			u.Protocol,
			u.HealthPath,
			u.LBStrategy,
			u.Enabled,
		).Scan(
			&u.ID,
			&u.CreatedAt,
			&u.UpdatedAt,
		)

		if err != nil {
			if isUniqueViolation(err) {
				return ErrUpstreamNameConflict
			}
			return fmt.Errorf(
				"create upstream '%s' for project %s: %w",
				u.Name,
				tc.ProjectID,
				err,
			)
		}

		return nil
	})
}

func (r *UpstreamRepo) GetByID(ctx context.Context, id string) (*model.Upstream, error) {
	var u model.Upstream

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			SELECT id, project_id, name, target_url, protocol,
			       COALESCE(health_path, ''),
			       lb_strategy::text,
			       enabled, created_at, updated_at
			FROM upstreams
			WHERE id = $1
			  AND project_id = $2
			  AND deleted_at IS NULL
		`

		return tx.QueryRowContext(ctx, q, id, tc.ProjectID).Scan(
			&u.ID,
			&u.ProjectID,
			&u.Name,
			&u.TargetURL,
			&u.Protocol,
			&u.HealthPath,
			&u.LBStrategy,
			&u.Enabled,
			&u.CreatedAt,
			&u.UpdatedAt,
		)
	})

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUpstreamNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetByID upstream %s: %w", id, err)
	}

	return &u, nil
}

// ListAllEnabledGlobal returns all enabled upstreams across all projects for
// gateway route loading. It bypasses tenant context and queries directly.
func (r *UpstreamRepo) ListAllEnabledGlobal(ctx context.Context) ([]model.Upstream, error) {
	const q = `
		SELECT id, project_id, name, target_url, protocol,
		       COALESCE(health_path, ''),
		       lb_strategy::text,
		       enabled, created_at, updated_at
		FROM upstreams
		WHERE enabled    = TRUE
		  AND deleted_at IS NULL
		ORDER BY name ASC
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list all enabled upstreams: %w", err)
	}
	defer rows.Close()

	var upstreams []model.Upstream
	for rows.Next() {
		var u model.Upstream
		if err := rows.Scan(
			&u.ID,
			&u.ProjectID,
			&u.Name,
			&u.TargetURL,
			&u.Protocol,
			&u.HealthPath,
			&u.LBStrategy,
			&u.Enabled,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan upstream row: %w", err)
		}
		upstreams = append(upstreams, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream rows: %w", err)
	}

	return upstreams, nil
}

func (r *UpstreamRepo) ListAll(ctx context.Context, limit, offset int) ([]model.Upstream, int, error) {
	var upstreams []model.Upstream
	var total int

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM upstreams WHERE project_id = $1 AND deleted_at IS NULL", tc.ProjectID).Scan(&total)
		if err != nil {
			return fmt.Errorf("count upstreams: %w", err)
		}

		const baseQ = `
			SELECT id, project_id, name, target_url, protocol,
			       COALESCE(health_path, ''),
			       lb_strategy::text,
			       enabled, created_at, updated_at
			FROM upstreams
			WHERE project_id = $1
			  AND deleted_at IS NULL
			ORDER BY name ASC
		`
		q := baseQ
		if limit > 0 {
			q += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
		}

		rows, err := tx.QueryContext(ctx, q, tc.ProjectID)
		if err != nil {
			return fmt.Errorf("query upstreams for project %s: %w", tc.ProjectID, err)
		}
		defer rows.Close()

		for rows.Next() {
			var u model.Upstream
			if err := rows.Scan(
				&u.ID,
				&u.ProjectID,
				&u.Name,
				&u.TargetURL,
				&u.Protocol,
				&u.HealthPath,
				&u.LBStrategy,
				&u.Enabled,
				&u.CreatedAt,
				&u.UpdatedAt,
			); err != nil {
				return fmt.Errorf("scan upstream row: %w", err)
			}
			upstreams = append(upstreams, u)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, 0, err
	}
	return upstreams, total, nil
}

func (r *UpstreamRepo) Update(ctx context.Context, id string, u *model.Upstream) error {
	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			UPDATE upstreams
			SET    name        = $3,
			       target_url  = $4,
			       protocol    = $5,
			       health_path = $6,
			       lb_strategy = $7,
			       enabled     = $8,
			       updated_at  = NOW()
			WHERE  id = $1
			  AND  project_id = $2
			  AND  deleted_at IS NULL
			RETURNING id, project_id, created_at, updated_at
		`

		return tx.QueryRowContext(
			ctx, q,
			id, tc.ProjectID,
			u.Name, u.TargetURL, u.Protocol, u.HealthPath, u.LBStrategy, u.Enabled,
		).Scan(&u.ID, &u.ProjectID, &u.CreatedAt, &u.UpdatedAt)
	})

	if errors.Is(err, sql.ErrNoRows) {
		return ErrUpstreamNotFound
	}
	if err != nil {
		return fmt.Errorf("Update upstream %s: %w", id, err)
	}

	r.logger.Info().Str("upstream_id", id).Str("name", u.Name).Msg("upstream updated successfully")
	return nil
}

func (r *UpstreamRepo) Disable(ctx context.Context, id string) error {
	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			UPDATE upstreams
			SET    enabled    = FALSE,
			       updated_at = NOW()
			WHERE  id = $1
			  AND  project_id = $2
			  AND  deleted_at IS NULL
		`

		res, err := tx.ExecContext(ctx, q, id, tc.ProjectID)
		if err != nil {
			return err
		}

		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("get rows affected: %w", err)
		}
		if n == 0 {
			return ErrUpstreamNotFound
		}
		return nil
	})

	if errors.Is(err, ErrUpstreamNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("Disable upstream %s: %w", id, err)
	}

	r.logger.Info().Str("upstream_id", id).Msg("upstream disabled successfully")
	return nil
}

// Delete performs a soft-delete and cleans up orphan references.
func (r *UpstreamRepo) Delete(ctx context.Context, id string) error {
	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		res, err := tx.ExecContext(ctx, `
			UPDATE upstreams
			SET    deleted_at = NOW(),
			       updated_at = NOW()
			WHERE  id = $1
			  AND  project_id = $2
			  AND  deleted_at IS NULL
		`, id, tc.ProjectID)
		if err != nil {
			return err
		}

		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("get rows affected: %w", err)
		}
		if n == 0 {
			return ErrUpstreamNotFound
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE routes
			SET    upstream_id = NULL,
			       updated_at  = NOW()
			WHERE  upstream_id = $1
			  AND  project_id  = $2
			  AND  deleted_at  IS NULL
		`, id, tc.ProjectID)
		if err != nil {
			return fmt.Errorf("clear route upstream ref: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE upstream_targets
			SET    deleted_at = NOW()
			WHERE  upstream_id = $1
			  AND  deleted_at  IS NULL
		`, id)
		if err != nil {
			return fmt.Errorf("soft-delete upstream targets: %w", err)
		}

		return nil
	})

	if errors.Is(err, ErrUpstreamNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("Delete upstream %s: %w", id, err)
	}

	r.logger.Info().Str("upstream_id", id).Msg("upstream deleted successfully (soft-delete)")
	return nil
}
