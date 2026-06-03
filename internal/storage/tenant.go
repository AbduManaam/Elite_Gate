package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// ErrNotFound is returned when a resource does not exist in the tenant scope.
var ErrNotFound = errors.New("resource not found")

// ErrForbidden is returned when a user does not have access to a project.
var ErrForbidden = errors.New("access denied")

// TenantContext carries validated project identity through the request lifecycle.
type TenantContext struct {
	ProjectID uuid.UUID
	UserID    uuid.UUID
	UserRole  string
}

type ctxKey string

const tenantCtxKey ctxKey = "tenant_ctx"

func WithTenantContext(ctx context.Context, tc TenantContext) context.Context {
	return context.WithValue(ctx, tenantCtxKey, tc)
}

func TenantFromContext(ctx context.Context) (TenantContext, error) {
	tc, ok := ctx.Value(tenantCtxKey).(TenantContext)
	if !ok || tc.ProjectID == uuid.Nil {
		return TenantContext{}, fmt.Errorf("tenant context not set in request: %w", ErrForbidden)
	}
	return tc, nil
}

type BaseRepo struct {
	db     *sql.DB
	logger zerolog.Logger
}

// setTenantSession sets the PostgreSQL session variables used by RLS policies.
func (r *BaseRepo) setTenantSession(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, userID uuid.UUID) error {
	r.logger.Trace().
		Str("project_id", projectID.String()).
		Str("user_id", userID.String()).
		Msg("setting postgres tenant transaction variables")

	_, err := tx.ExecContext(ctx,
		`SELECT
			set_config('app.project_id', $1::text, TRUE),
			set_config('app.current_user_id', $2::text, TRUE)`,
		projectID.String(),
		userID.String(),
	)
	if err != nil {
		r.logger.Error().Err(err).
			Str("project_id", projectID.String()).
			Msg("failed to set postgres tenant session variables")
		return fmt.Errorf("setTenantSession: %w", err)
	}
	return nil
}

// withTenantTx opens a transaction, sets the session context, executes the callback, and commits/rolls back.
func (r *BaseRepo) withTenantTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tc, err := TenantFromContext(ctx)
	if err != nil {
		r.logger.Warn().Err(err).Msg("tenant context extraction failed")
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		r.logger.Error().Err(err).Msg("failed to start database transaction")
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // Safe: no-op if transaction has committed

	if err := r.setTenantSession(ctx, tx, tc.ProjectID, tc.UserID); err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		r.logger.Debug().Err(err).
			Str("project_id", tc.ProjectID.String()).
			Msg("database transaction aborted due to operation error")
		return err
	}

	if err := tx.Commit(); err != nil {
		r.logger.Error().Err(err).
			Str("project_id", tc.ProjectID.String()).
			Msg("failed to commit transaction")
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

type MembershipRepo struct {
	BaseRepo
}

func NewMembershipRepo(db *sql.DB, logger zerolog.Logger) *MembershipRepo {
	return &MembershipRepo{BaseRepo{db: db, logger: logger}}
}

// ValidateMembership verifies project-user relationship.
func (r *MembershipRepo) ValidateMembership(ctx context.Context, projectID, userID uuid.UUID) (string, error) {
	r.logger.Debug().
		Str("project_id", projectID.String()).
		Str("user_id", userID.String()).
		Msg("validating project membership")

	const q = `
		SELECT pm.role
		FROM   project_members pm
		JOIN   projects         p  ON p.id = pm.project_id
		JOIN   admin_users      au ON au.id = pm.admin_user_id
		WHERE  pm.project_id    = $1
		  AND  pm.admin_user_id = $2
		  AND  p.deleted_at     IS NULL
		  AND  au.deleted_at    IS NULL
		  AND  au.is_active     = TRUE
	`
	var role string
	err := r.db.QueryRowContext(ctx, q, projectID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		r.logger.Warn().
			Str("project_id", projectID.String()).
			Str("user_id", userID.String()).
			Msg("user is not a member of project or project/user is inactive")
		return "", ErrForbidden
	}
	if err != nil {
		r.logger.Error().Err(err).
			Str("project_id", projectID.String()).
			Msg("membership validation query failed")
		return "", fmt.Errorf("ValidateMembership: %w", err)
	}

	r.logger.Debug().
		Str("project_id", projectID.String()).
		Str("user_id", userID.String()).
		Str("role", role).
		Msg("membership validation successful")
	return role, nil
}
