package storage

import (
	"context"
	"database/sql"

	"elitegate/internal/model"
)

type UpstreamRepo struct {
	db *sql.DB
}

func NewUpstreamRepo(db *sql.DB) *UpstreamRepo {
	return &UpstreamRepo{db: db}
}

func (r *UpstreamRepo) ListAll(ctx context.Context) ([]model.Upstream, error) {
	const q = `
		SELECT id, name, target_url, protocol, COALESCE(health_path,''),
		       enabled, created_at, updated_at
		FROM upstreams
		ORDER BY name ASC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Upstream
	for rows.Next() {
		var u model.Upstream
		if err := rows.Scan(
			&u.ID, &u.Name, &u.TargetURL, &u.Protocol, &u.HealthPath,
			&u.Enabled, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UpstreamRepo) Create(ctx context.Context, u *model.Upstream) error {
	const q = `
		INSERT INTO upstreams (name, target_url, protocol, health_path, enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, q,
		u.Name, u.TargetURL, u.Protocol, u.HealthPath, u.Enabled,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}