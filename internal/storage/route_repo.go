package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/rs/zerolog"

	"elitegate/internal/model"
)

type RouteRepo struct {
	BaseRepo
}

func NewRouteRepo(db *sql.DB, logger zerolog.Logger) *RouteRepo {
	return &RouteRepo{BaseRepo{db: db, logger: logger}}
}

const listQuery = `
	SELECT
		r.id,
		r.project_id,
		r.name,
		r.path,
		r.upstream_id,
		COALESCE(u.target_url, '')   AS upstream_url,
		COALESCE(u.protocol, 'http') AS protocol,
		r.methods,
		r.match_type,
		r.enabled,
		r.policy_id,
		COALESCE(p.auth_required, TRUE)   AS auth_required,
		COALESCE(p.rate_limit_rpm, 0)     AS rate_limit_rpm,
		COALESCE(p.allowed_origins, '{}') AS allowed_origins,
		COALESCE(p.allowed_roles,   '{}') AS allowed_roles,
		COALESCE(p.allowed_scopes,  '{}') AS allowed_scopes,
		r.created_at,
		r.updated_at
	FROM   routes r
	LEFT JOIN upstreams u ON u.id = r.upstream_id AND u.deleted_at IS NULL
	LEFT JOIN policies  p ON p.id = r.policy_id AND p.deleted_at IS NULL
`

func (r *RouteRepo) ListEnabled(ctx context.Context) ([]model.Route, error) {
	// If no tenant context is present (e.g. gateway router loading all routes globally),
	// we bypass the tenant transaction wrapper and query everything.
	tc, err := TenantFromContext(ctx)
	if err != nil {
		r.logger.Trace().Msg("ListEnabled: no tenant context, listing all enabled routes globally")
		q := listQuery + ` WHERE r.enabled = TRUE AND r.deleted_at IS NULL ORDER BY length(r.path) DESC`
		rows, err := r.db.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanRoutes(rows)
	}

	r.logger.Debug().Str("project_id", tc.ProjectID.String()).Msg("ListEnabled: tenant context found, listing isolated enabled routes")
	var routes []model.Route
	err = r.withTenantTx(ctx, func(tx *sql.Tx) error {
		q := listQuery + ` WHERE r.enabled = TRUE AND r.deleted_at IS NULL ORDER BY length(r.path) DESC`
		rows, err := tx.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		routes, err = scanRoutes(rows)
		return err
	})
	if err != nil {
		return nil, err
	}
	return routes, nil
}

func (r *RouteRepo) GetByID(ctx context.Context, id string) (*model.Route, error) {
	var rt model.Route

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		q := listQuery + ` WHERE r.id = $1 AND r.project_id = $2 AND r.deleted_at IS NULL`
		row := tx.QueryRowContext(ctx, q, id, tc.ProjectID)
		rt, err = scanRoute(row)
		return err
	})

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRouteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetByID route %s: %w", id, err)
	}

	return &rt, nil
}
func (r *RouteRepo) ListAll(ctx context.Context, limit, offset int) ([]model.Route, int, error) {
	tc, err := TenantFromContext(ctx)
	if err != nil {
		r.logger.Trace().Msg("ListAll: no tenant context, listing all routes globally")
		var total int
		err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM routes WHERE deleted_at IS NULL").Scan(&total)
		if err != nil {
			return nil, 0, err
		}

		q := listQuery + ` WHERE r.deleted_at IS NULL ORDER BY r.path ASC`
		if limit > 0 {
			q += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
		}
		rows, err := r.db.QueryContext(ctx, q)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		routes, err := scanRoutes(rows)
		return routes, total, err
	}

	r.logger.Debug().Str("project_id", tc.ProjectID.String()).Msg("ListAll: tenant context found, listing isolated routes")
	var routes []model.Route
	var total int
	err = r.withTenantTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM routes WHERE deleted_at IS NULL").Scan(&total)
		if err != nil {
			return err
		}

		q := listQuery + ` WHERE r.deleted_at IS NULL ORDER BY r.path ASC`
		if limit > 0 {
			q += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
		}
		rows, err := tx.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		routes, err = scanRoutes(rows)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	return routes, total, nil
}

func (r *RouteRepo) Create(ctx context.Context, rt *model.Route) error {
	r.logger.Info().Str("path", rt.Path).Msg("Create: initiating route creation")

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return err
		}
		rt.ProjectID = tc.ProjectID.String()

		if rt.Name == "" {
			rt.Name = "route_" + uuid.New().String()[:8]
		}

		const q = `
			INSERT INTO routes (project_id, name, path, upstream_id, policy_id, match_type, enabled, methods)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, created_at, updated_at
		`
		err = tx.QueryRowContext(ctx, q,
			tc.ProjectID, rt.Name, rt.Path, rt.UpstreamID, rt.PolicyID, rt.MatchType, rt.Enabled, pq.Array(rt.Methods),
		).Scan(&rt.ID, &rt.CreatedAt, &rt.UpdatedAt)
		if err != nil {
			if isUniqueViolation(err) {
				return ErrRouteNameConflict
			}
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	r.logger.Info().Str("route_id", rt.ID).Str("project_id", rt.ProjectID).Msg("Create: route created successfully")
	return nil
}

func (r *RouteRepo) Update(ctx context.Context, id string, rt *model.Route) error {
	r.logger.Info().Str("route_id", id).Str("path", rt.Path).Msg("Update: initiating route update")

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		const q = `
			UPDATE routes
			SET    name         = $2,
			       path         = $3,
			       upstream_id  = $4,
			       policy_id    = $5,
			       match_type   = $6,
			       enabled      = $7,
			       methods      = $8,
			       updated_at   = NOW()
			WHERE  id = $1 AND deleted_at IS NULL
			RETURNING id, updated_at
		`
		err := tx.QueryRowContext(ctx, q,
			id, rt.Name, rt.Path, rt.UpstreamID, rt.PolicyID, rt.MatchType, rt.Enabled, pq.Array(rt.Methods),
		).Scan(&rt.ID, &rt.UpdatedAt)
		if err == sql.ErrNoRows {
			return ErrRouteNotFound
		}
		return err
	})

	if err != nil {
		return err
	}

	r.logger.Info().Str("route_id", id).Msg("Update: route updated successfully")
	return nil
}

func (r *RouteRepo) Delete(ctx context.Context, id string) error {
	r.logger.Info().Str("route_id", id).Msg("Delete: initiating route deletion")

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE routes SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrRouteNotFound
		}
		return nil
	})

	if err != nil {
		return err
	}

	r.logger.Info().Str("route_id", id).Msg("Delete: route deleted successfully (soft-delete)")
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
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
		&rt.ID,
		&rt.ProjectID,
		&rt.Name,
		&rt.Path,
		&upstreamID,
		&rt.UpstreamURL,
		&rt.Protocol,
		pq.Array(&rt.Methods),
		&rt.MatchType,
		&rt.Enabled,
		&policyID,
		&rt.AuthRequired,
		&rt.RateLimitRPM,
		pq.Array(&rt.AllowedOrigins),
		pq.Array(&rt.AllowedRoles),
		pq.Array(&rt.AllowedScopes),
		&rt.CreatedAt,
		&rt.UpdatedAt,
	)
	if upstreamID.Valid {
		rt.UpstreamID = &upstreamID.String
	}
	if policyID.Valid {
		rt.PolicyID = &policyID.String
	}
	return rt, err
}

func (r *RouteRepo) Disable(ctx context.Context, id string) error {
	r.logger.Info().Str("route_id", id).Msg("Disable: initiating route disabling")

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			UPDATE routes
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
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrRouteNotFound
		}
		return nil
	})

	if err != nil {
		return err
	}

	r.logger.Info().Str("route_id", id).Msg("Disable: route disabled successfully")
	return nil
}

func (r *RouteRepo) Enable(ctx context.Context, id string) error {
	r.logger.Info().Str("route_id", id).Msg("Enable: initiating route enabling")

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			UPDATE routes
			SET    enabled    = TRUE,
			       updated_at = NOW()
			WHERE  id = $1
			  AND  project_id = $2
			  AND  deleted_at IS NULL
		`
		res, err := tx.ExecContext(ctx, q, id, tc.ProjectID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrRouteNotFound
		}
		return nil
	})

	if err != nil {
		return err
	}

	r.logger.Info().Str("route_id", id).Msg("Enable: route enabled successfully")
	return nil
}


var (
	ErrRouteNotFound     = errors.New("route not found")
	ErrRouteNameConflict = errors.New("route name already exists")
)
