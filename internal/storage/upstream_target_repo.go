package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"elitegate/internal/model"

	"github.com/rs/zerolog"
)

var ErrUpstreamTargetNotFound = errors.New("upstream target not found")

type UpstreamTargetRepo struct {
	BaseRepo
}

func NewUpstreamTargetRepo(db *sql.DB, Logger zerolog.Logger) *UpstreamTargetRepo {
	return &UpstreamTargetRepo{
		BaseRepo{db: db, logger: Logger},
	}
}

// Add inserts a new backend target into an upstream's pool by Admin User
func (r *UpstreamTargetRepo) Add(ctx context.Context, t *model.UpstreamTarget) error {
	return r.withTenantTx(ctx, func(tx *sql.Tx) error {
		const q = `
			INSERT INTO upstream_targets (upstream_id, target_url, weight, enabled)
			VALUES ($1, $2, $3, $4)
			RETURNING id, created_at, updated_at
		`
		err := tx.QueryRowContext(ctx, q, t.UpstreamID, t.TargetURL, t.Weight, t.Enabled).
			Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return fmt.Errorf("add upstream target: %w", err)
		}
		return nil
	})
}

// ListByUpstream returns all enabled, non-deleted targets for one upstream,
// scoped to the current tenant via RLS on the parent upstreams row.
func (r *UpstreamTargetRepo) ListByUpstream(ctx context.Context, upstreamID string) ([]model.UpstreamTarget, error) {
	var targets []model.UpstreamTarget

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		const q = `
			SELECT ut.id, ut.upstream_id, ut.target_url, ut.weight, ut.enabled,
			       ut.created_at, ut.updated_at
			FROM   upstream_targets ut
			JOIN   upstreams u ON u.id = ut.upstream_id
			WHERE  ut.upstream_id = $1
			  AND  ut.deleted_at IS NULL
			  AND  u.deleted_at  IS NULL
			ORDER  BY ut.created_at ASC
		`
		rows, err := tx.QueryContext(ctx, q, upstreamID)
		if err != nil {
			return fmt.Errorf("list upstream targets: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var t model.UpstreamTarget
			if err := rows.Scan(
				&t.ID, &t.UpstreamID, &t.TargetURL, &t.Weight, &t.Enabled,
				&t.CreatedAt, &t.UpdatedAt,
			); err != nil {
				return fmt.Errorf("scan upstream target: %w", err)
			}
			targets = append(targets, t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return targets, nil
}

// ListAllEnabledGlobal returns all enabled targets for gateway route loading.
func (r *UpstreamTargetRepo) ListAllEnabledGlobal(ctx context.Context) (map[string][]model.UpstreamTarget, error) {
	const q = `
		SELECT ut.id, ut.upstream_id, ut.target_url, ut.weight, ut.enabled,
		       ut.created_at, ut.updated_at
		FROM   upstream_targets ut
		JOIN   upstreams u ON u.id = ut.upstream_id
		WHERE  ut.deleted_at IS NULL
		  AND  ut.enabled    = TRUE
		  AND  u.deleted_at  IS NULL
		ORDER  BY ut.upstream_id, ut.created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list all enabled upstream targets: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]model.UpstreamTarget)
	for rows.Next() {
		var t model.UpstreamTarget
		if err := rows.Scan(
			&t.ID, &t.UpstreamID, &t.TargetURL, &t.Weight, &t.Enabled,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan upstream target: %w", err)
		}
		out[t.UpstreamID] = append(out[t.UpstreamID], t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream targets: %w", err)
	}
	return out, nil
}

func (r *UpstreamTargetRepo) Remove(ctx context.Context, id string) error {
	return r.withTenantTx(ctx, func(tx *sql.Tx) error {
		const q = `
			UPDATE upstream_targets
			SET    deleted_at = NOW(), updated_at = NOW()
			WHERE  id = $1 AND deleted_at IS NULL
		`
		res, err := tx.ExecContext(ctx, q, id)
		if err != nil {
			return fmt.Errorf("remove upstream target: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrUpstreamTargetNotFound
		}
		return nil
	})
}
