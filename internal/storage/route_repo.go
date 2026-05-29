package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"

	"elitegate/internal/model"
)

type RouteRepo struct {
	db *sql.DB
}

func NewRouteRepo(db *sql.DB) *RouteRepo {
	return &RouteRepo{db: db}
}

func (r *RouteRepo) ListEnabled(ctx context.Context) ([]model.Route, error) {
	const q = `
		SELECT id, path, upstream_url, upstream_id, methods, protocol,
		       match_type, enabled, auth_required, rate_limit_rpm,
		       created_at, updated_at
		FROM routes
		WHERE enabled = TRUE
		ORDER BY length(path) DESC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Route
	for rows.Next() {
		rt, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

func (r *RouteRepo) ListAll(ctx context.Context) ([]model.Route, error) {
	const q = `
		SELECT id, path, upstream_url, upstream_id, methods, protocol,
		       match_type, enabled, auth_required, rate_limit_rpm,
		       created_at, updated_at
		FROM routes
		ORDER BY path ASC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Route
	for rows.Next() {
		rt, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

func (r *RouteRepo) Create(ctx context.Context, rt *model.Route) error {
	const q = `
		INSERT INTO routes (path, upstream_url, methods, protocol, match_type, enabled, auth_required, rate_limit_rpm)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, q,
		rt.Path, rt.UpstreamURL, pq.Array(rt.Methods), rt.Protocol,
		rt.MatchType, rt.Enabled, rt.AuthRequired, rt.RateLimitRPM,
	).Scan(&rt.ID, &rt.CreatedAt, &rt.UpdatedAt)
}

func (r *RouteRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM routes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRoute(s rowScanner) (model.Route, error) {
	var rt model.Route
	var upstreamID sql.NullString
	err := s.Scan(
		&rt.ID, &rt.Path, &rt.UpstreamURL, &upstreamID, pq.Array(&rt.Methods),
		&rt.Protocol, &rt.MatchType, &rt.Enabled, &rt.AuthRequired, &rt.RateLimitRPM,
		&rt.CreatedAt, &rt.UpdatedAt,
	)
	if upstreamID.Valid {
		rt.UpstreamID = &upstreamID.String
	}
	return rt, err
}

var ErrRouteNotFound = errors.New("route not found")