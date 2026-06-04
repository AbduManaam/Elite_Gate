package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"elitegate/internal/model"
)

type PolicyRepo struct {
	BaseRepo
}

func NewPolicyRepo(db *sql.DB) *PolicyRepo {
	return &PolicyRepo{BaseRepo{db: db}}
}

func (r *PolicyRepo) ListAll(ctx context.Context) ([]model.Policy, error) {
	var policies []model.Policy

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			SELECT id, project_id, name, auth_required, rate_limit_rpm, created_at, updated_at
			FROM policies
			WHERE project_id = $1 AND deleted_at IS NULL
			ORDER BY name ASC
		`

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

		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate policy rows: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return policies, nil
}

func (r *PolicyRepo) GetByID(ctx context.Context, id string) (*model.Policy, error) {
	var p model.Policy

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			SELECT id, project_id, name, auth_required, rate_limit_rpm, created_at, updated_at
			FROM policies
			WHERE id = $1
			  AND project_id = $2
			  AND deleted_at IS NULL
		`

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

		const q = `
			INSERT INTO policies (project_id, name, auth_required, rate_limit_rpm)
			VALUES ($1, $2, $3, $4)
			RETURNING id, created_at, updated_at
		`

		err = tx.QueryRowContext(
			ctx,
			q,
			tc.ProjectID,
			p.Name,
			p.AuthRequired,
			p.RateLimitRPM,
		).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

		if err != nil {
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

		const q = `
			UPDATE policies
			SET    name           = $3,
			       auth_required  = $4,
			       rate_limit_rpm = $5,
			       updated_at     = NOW()
			WHERE  id = $1
			   AND project_id = $2
			   AND deleted_at IS NULL
			RETURNING updated_at
		`

		err = tx.QueryRowContext(
			ctx,
			q,
			id,
			tc.ProjectID,
			p.Name,
			p.AuthRequired,
			p.RateLimitRPM,
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
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		return model.Policy{}, err
	}

	return p, nil
}

var ErrPolicyNotFound = errors.New("policy not found")