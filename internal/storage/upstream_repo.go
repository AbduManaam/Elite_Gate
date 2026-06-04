package storage

import (
	"context"
	"database/sql"
	"fmt"

	"elitegate/internal/model"
)

type UpstreamRepo struct {
	BaseRepo
}

func NewUpstreamRepo(db *sql.DB) *UpstreamRepo {
	return &UpstreamRepo{BaseRepo{db: db}}
}

func (r *UpstreamRepo) ListAll(ctx context.Context) ([]model.Upstream, error) {
	var upstreams []model.Upstream

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			SELECT id, project_id, name, target_url, protocol,
			       COALESCE(health_path, ''),
			       enabled, created_at, updated_at
			FROM upstreams
			WHERE project_id = $1
			  AND deleted_at IS NULL
			ORDER BY name ASC
		`

		rows, err := tx.QueryContext(ctx, q, tc.ProjectID)
		if err != nil {
			return fmt.Errorf("query upstreams for project %s: %w",
				tc.ProjectID, err)
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
				&u.Enabled,
				&u.CreatedAt,
				&u.UpdatedAt,
			); err != nil {
				return fmt.Errorf("scan upstream row: %w", err)
			}

			upstreams = append(upstreams, u)
		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate upstream rows: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("ListAll: %w", err)
	}

	return upstreams, nil
}

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
				enabled
			)
			VALUES ($1, $2, $3, $4, $5, $6)
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
			u.Enabled,
		).Scan(
			&u.ID,
			&u.CreatedAt,
			&u.UpdatedAt,
		)

		if err != nil {
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