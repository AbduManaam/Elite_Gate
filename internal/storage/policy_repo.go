package storage

import (
	"context"
	"database/sql"
	"errors"

	"elitegate/internal/model"
)

type PolicyRepo struct {
	db *sql.DB
}

func NewPolicyRepo(db *sql.DB) *PolicyRepo {
	return &PolicyRepo{db: db}
}

func (r *PolicyRepo) ListAll(ctx context.Context) ([]model.Policy, error) {
	const q = `
		SELECT id, name, auth_required, rate_limit_rpm, created_at, updated_at
		FROM   policies
		ORDER  BY name ASC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PolicyRepo) GetByID(ctx context.Context, id string) (*model.Policy, error) {
	const q = `
		SELECT id, name, auth_required, rate_limit_rpm, created_at, updated_at
		FROM   policies
		WHERE  id = $1
	`
	p, err := scanPolicy(r.db.QueryRowContext(ctx, q, id))
	if err == sql.ErrNoRows {
		return nil, ErrPolicyNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PolicyRepo) Create(ctx context.Context, p *model.Policy) error {
	const q = `
		INSERT INTO policies (name, auth_required, rate_limit_rpm)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, q,
		p.Name, p.AuthRequired, p.RateLimitRPM,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *PolicyRepo) Update(ctx context.Context, id string, p *model.Policy) error {
	const q = `
		UPDATE policies
		SET    name           = $2,
		       auth_required  = $3,
		       rate_limit_rpm = $4,
		       updated_at     = NOW()
		WHERE  id = $1
		RETURNING updated_at
	`
	err := r.db.QueryRowContext(ctx, q,
		id, p.Name, p.AuthRequired, p.RateLimitRPM,
	).Scan(&p.UpdatedAt)
	if err == sql.ErrNoRows {
		return ErrPolicyNotFound
	}
	return err
}

func (r *PolicyRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM policies WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPolicyNotFound
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

type policyScanner interface {
	Scan(dest ...any) error
}

func scanPolicy(s policyScanner) (model.Policy, error) {
	var p model.Policy
	err := s.Scan(&p.ID, &p.Name, &p.AuthRequired, &p.RateLimitRPM, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

var ErrPolicyNotFound = errors.New("policy not found")
