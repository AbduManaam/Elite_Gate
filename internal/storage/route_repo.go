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

// listQuery is shared by ListEnabled and ListAll.
// JOINs upstreams → target_url + protocol
// JOINs policies  → auth_required + rate_limit_rpm
// Subquery        → methods from route_methods table
// COALESCE keeps backward compat with rows that still have old columns populated.
const listQuery = `
	SELECT
		r.id,
		r.path,
		r.upstream_id,
		COALESCE(u.target_url, '')   AS upstream_url,
		COALESCE(u.protocol, 'http') AS protocol,
		ARRAY(
			SELECT rm.method
			FROM   route_methods rm
			WHERE  rm.route_id = r.id
		)                            AS methods,
		r.match_type,
		r.enabled,
		r.policy_id,
		COALESCE(p.auth_required, TRUE)  AS auth_required,
		COALESCE(p.rate_limit_rpm, 0)    AS rate_limit_rpm,
		r.created_at,
		r.updated_at
	FROM   routes r
	LEFT JOIN upstreams u ON u.id = r.upstream_id
	LEFT JOIN policies  p ON p.id = r.policy_id
`

func (r *RouteRepo) ListEnabled(ctx context.Context) ([]model.Route, error) {
	q := listQuery + `WHERE r.enabled = TRUE ORDER BY length(r.path) DESC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRoutes(rows)
}

func (r *RouteRepo) ListAll(ctx context.Context) ([]model.Route, error) {
	q := listQuery + `ORDER BY r.path ASC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRoutes(rows)
}

// Create inserts a new route and its methods in a single transaction.
// Writes upstream_id + policy_id (new schema).
// Also writes upstream_url for backward compat until migration 0008 runs.
func (r *RouteRepo) Create(ctx context.Context, rt *model.Route) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const q = `
		INSERT INTO routes (path, upstream_id, policy_id, match_type, enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`
	err = tx.QueryRowContext(ctx, q,
		rt.Path, rt.UpstreamID, rt.PolicyID, rt.MatchType, rt.Enabled,
	).Scan(&rt.ID, &rt.CreatedAt, &rt.UpdatedAt)
	if err != nil {
		return err
	}

	if err := insertMethods(ctx, tx, rt.ID, rt.Methods); err != nil {
		return err
	}

	return tx.Commit()
}

// Update replaces a route's fields and rebuilds its method list atomically.
func (r *RouteRepo) Update(ctx context.Context, id string, rt *model.Route) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const q = `
		UPDATE routes
		SET    path         = $2,
		       upstream_id  = $3,
		       policy_id    = $4,
		       match_type   = $5,
		       enabled      = $6,
		       updated_at   = NOW()
		WHERE  id = $1
		RETURNING id, updated_at
	`
	err = tx.QueryRowContext(ctx, q,
		id, rt.Path, rt.UpstreamID, rt.PolicyID, rt.MatchType, rt.Enabled,
	).Scan(&rt.ID, &rt.UpdatedAt)
	if err == sql.ErrNoRows {
		return ErrRouteNotFound
	}
	if err != nil {
		return err
	}

	// Rebuild method list: delete old rows, insert new ones.
	if _, err := tx.ExecContext(ctx, `DELETE FROM route_methods WHERE route_id = $1`, id); err != nil {
		return err
	}
	if err := insertMethods(ctx, tx, id, rt.Methods); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *RouteRepo) Delete(ctx context.Context, id string) error {
	// route_methods rows are deleted automatically via ON DELETE CASCADE.
	res, err := r.db.ExecContext(ctx, `DELETE FROM routes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrRouteNotFound
	}
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...any) error
}

type txExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertMethods(ctx context.Context, tx *sql.Tx, routeID string, methods []string) error {
	for _, m := range methods {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO route_methods (route_id, method) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			routeID, m,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func scanRoutes(rows *sql.Rows) ([]model.Route, error) {
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

func scanRoute(s rowScanner) (model.Route, error) {
	var rt model.Route
	var upstreamID sql.NullString
	var policyID sql.NullString

	err := s.Scan(
		&rt.ID, &rt.Path,
		&upstreamID,
		&rt.UpstreamURL,
		&rt.Protocol,
		pq.Array(&rt.Methods),
		&rt.MatchType, &rt.Enabled,
		&policyID,
		&rt.AuthRequired, &rt.RateLimitRPM,
		&rt.CreatedAt, &rt.UpdatedAt,
	)
	if upstreamID.Valid {
		rt.UpstreamID = &upstreamID.String
	}
	if policyID.Valid {
		rt.PolicyID = &policyID.String
	}
	return rt, err
}

var ErrRouteNotFound = errors.New("route not found")