package storage

// THIS FILE

// It manages Multi-Tenant Isolation and Row-Level Security (RLS):

// Context Management: Passes tenant details (ProjectID, UserID) through Go request contexts.
// Session Security: Automatically sets app.project_id inside PostgreSQL transactions so the database can filter rows for that project only.
// Membership Check: Validates if a user actually belongs to a project before letting them query it.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var ErrNotFound = errors.New("resource not found")
var ErrForbidden = errors.New("access denied")

// A struct carrying the identity of the current project (ProjectID), the active user (UserID), and their permission role (UserRole).
type TenantContext struct {
	ProjectID uuid.UUID
	UserID    uuid.UUID
	UserRole  string
}

//---------------------------------------------------------------------------------------------------------------

// WithTenantContext & TenantFromContext: Helper functions that securely store and retrieve the TenantContext to/from
// the Go context.Context object. They use a private key type ctxKey to prevent name collisions in the context map.

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

//---------------------------------------------------------------------------------------------------------------

// BaseRepo contains common database functions
// used by all repositories.

type BaseRepo struct {
	db     *sql.DB
	logger zerolog.Logger
}

/*setTenantSession sets tenant information in PostgreSQL
for the current transaction only.

app.project_id      = which project (tenant) is being accessed
app.current_user_id = which user is making the request

TRUE means the value exists only for the current transaction
and is automatically removed when the transaction ends.

Flow:
1. setTenantSession() sets app.project_id
2. RLS policies use that value
3. The user can access only data from their own project*/
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

//---------------------------------------------------------------------------------------------------------------
/*
withTenantTx automatically performs all required setup before running a database query.

Get tenant information from the context
        ↓
Start a transaction
        ↓
Temporarily set the current project ID on the database session.
It is automatically cleared when the request finishes, preventing
data from different tenants from mixing. Without a transaction,
the value could remain on the connection.
        ↓
Set app.project_id by calling setTenantSession()
RLS policies use values such as app.project_id and app.current_user_id.
        ↓
Run the query by calling the callback function fn(tx)
Examples: routeRepo.Create(tx, route), routeRepo.List(), routeRepo.Update()
        ↓
Success? ── Yes → Commit transaction
        │
        No
        ↓
     Rollback transaction

after this,the transaction ends, and PostgreSQL automatically removes those transaction-local settings.
bcz of setTenantSession() uses:SELECT set_config('app.project_id', 'project-123', TRUE);
.SELECT set_config('app.current_user_id', 'user-456', TRUE);
the TRUE means:
Set this value only for the current transaction.*/
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

//---------------------------------------------------------------------------------------------------------------

// ValidateMembership: Queries the database to verify if a user has access to a specific project. If they are
// an active member, it returns their role (e.g., owner, editor, viewer); otherwise, it returns ErrForbidden.

type MembershipRepo struct {
	BaseRepo
}

func NewMembershipRepo(db *sql.DB, logger zerolog.Logger) *MembershipRepo {
	return &MembershipRepo{BaseRepo{db: db, logger: logger}}
}

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
