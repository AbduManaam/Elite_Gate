package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"elitegate/helper"
	"elitegate/internal/model"

	"github.com/lib/pq"
)

type PolicyRepo struct {
	BaseRepo
}

const selectFields = "id, project_id, name, auth_required, rate_limit_rpm, allowed_origins, allowed_roles, allowed_scopes, ip_allowlist, ip_blocklist, created_at, updated_at"

const insertQ = `
	INSERT INTO policies (project_id, name, auth_required, rate_limit_rpm, allowed_origins, allowed_roles, allowed_scopes, ip_allowlist, ip_blocklist)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	RETURNING id, created_at, updated_at
`

const updateQ = `
	UPDATE policies
	SET    name            = $3,
	       auth_required   = $4,
	       rate_limit_rpm  = $5,
	       allowed_origins = $6,
	       allowed_roles   = $7,
	       allowed_scopes  = $8,
	       ip_allowlist    = $9,
	       ip_blocklist    = $10,
	       updated_at      = NOW()
	WHERE  id = $1
	   AND project_id = $2
	   AND deleted_at IS NULL
	RETURNING updated_at
`

func NewPolicyRepo(db *sql.DB) *PolicyRepo {
	return &PolicyRepo{BaseRepo{db: db}}
}

func (r *PolicyRepo) ListAll(ctx context.Context, limit, offset int) ([]model.Policy, int, error) {
	var policies []model.Policy
	var total int

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM policies WHERE project_id = $1 AND deleted_at IS NULL", tc.ProjectID).Scan(&total)
		if err != nil {
			return fmt.Errorf("count policies: %w", err)
		}

		q := fmt.Sprintf(`
			SELECT %s
			FROM policies
			WHERE project_id = $1 AND deleted_at IS NULL
			ORDER BY name ASC`, selectFields)

		if limit > 0 {
			q += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
		}

		rows, err := tx.QueryContext(ctx, q, tc.ProjectID)
		if err != nil {
			return fmt.Errorf("query policies: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			p, err := scanPolicy(rows)
			if err != nil {
				return fmt.Errorf("scan policy row: %w", err)
			}
			policies = append(policies, p)
		}

		return rows.Err()
	})

	if err != nil {
		return nil, 0, err
	}

	return policies, total, nil
}

func (r *PolicyRepo) GetByID(ctx context.Context, id string) (*model.Policy, error) {
	var p model.Policy

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		q := fmt.Sprintf(`
			SELECT %s
			FROM policies
			WHERE id = $1
			  AND project_id = $2
			  AND deleted_at IS NULL`, selectFields)

		p, err = scanPolicy(tx.QueryRowContext(ctx, q, id, tc.ProjectID))
		if err != nil {
			return err
		}

		return nil
	})

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPolicyNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("get policy %s: %w", id, err)
	}

	return &p, nil
}

func (r *PolicyRepo) Create(ctx context.Context, p *model.Policy) error {
	return r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		p.ProjectID = tc.ProjectID.String()

		allowedOrigins := p.AllowedOrigins
		if allowedOrigins == nil {
			allowedOrigins = []string{}
		}

		err = tx.QueryRowContext(
			ctx,
			insertQ,
			tc.ProjectID,
			p.Name,
			p.AuthRequired,
			p.RateLimitRPM,
			pq.Array(helper.OrEmpty(p.AllowedOrigins)),
			pq.Array(helper.OrEmpty(p.AllowedRoles)),
			pq.Array(helper.OrEmpty(p.AllowedScopes)),
			pq.Array(helper.OrEmpty(p.IPAllowlist)),
			pq.Array(helper.OrEmpty(p.IPBlocklist)),
		).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

		if err != nil {
			if isUniqueViolation(err) {
				return ErrPolicyNameConflict
			}
			return fmt.Errorf("create policy: %w", err)
		}

		return nil
	})
}

func (r *PolicyRepo) Update(ctx context.Context, id string, p *model.Policy) error {
	return r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		allowedOrigins := p.AllowedOrigins
		if allowedOrigins == nil {
			allowedOrigins = []string{}
		}

		err = tx.QueryRowContext(
			ctx,
			updateQ,
			id,
			tc.ProjectID,
			p.Name,
			p.AuthRequired,
			p.RateLimitRPM,
			pq.Array(helper.OrEmpty(p.AllowedOrigins)),
			pq.Array(helper.OrEmpty(p.AllowedRoles)),
			pq.Array(helper.OrEmpty(p.AllowedScopes)),
			pq.Array(helper.OrEmpty(p.IPAllowlist)),
			pq.Array(helper.OrEmpty(p.IPBlocklist)),
		).Scan(&p.UpdatedAt)

		if errors.Is(err, sql.ErrNoRows) {
			return ErrPolicyNotFound
		}

		if err != nil {
			return fmt.Errorf("update policy %s: %w", id, err)
		}

		return nil
	})
}

func (r *PolicyRepo) Delete(ctx context.Context, id string) error {
	return r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		res, err := tx.ExecContext(
			ctx,
			`UPDATE policies
			 SET deleted_at = NOW()
			 WHERE id = $1
			   AND project_id = $2
			   AND deleted_at IS NULL`,
			id,
			tc.ProjectID,
		)
		if err != nil {
			return fmt.Errorf("delete policy %s: %w", id, err)
		}

		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("get rows affected: %w", err)
		}

		if n == 0 {
			return ErrPolicyNotFound
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE routes
			SET    policy_id  = NULL,
			       updated_at = NOW()
			WHERE  policy_id  = $1
			  AND  project_id = $2
			  AND  deleted_at IS NULL
		`, id, tc.ProjectID)
		if err != nil {
			return fmt.Errorf("clear route policy ref: %w", err)
		}

		return nil
	})
}

type policyScanner interface {
	Scan(dest ...any) error
}

func scanPolicy(s policyScanner) (model.Policy, error) {
	var p model.Policy

	err := s.Scan(
		&p.ID,
		&p.ProjectID,
		&p.Name,
		&p.AuthRequired,
		&p.RateLimitRPM,
		pq.Array(&p.AllowedOrigins),
		pq.Array(&p.AllowedRoles),
		pq.Array(&p.AllowedScopes),
		pq.Array(&p.IPAllowlist),
		pq.Array(&p.IPBlocklist),
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		return model.Policy{}, err
	}

	return p, nil
}

var (
	ErrPolicyNotFound     = errors.New("policy not found")
	ErrPolicyNameConflict = errors.New("policy name already exists")
)
