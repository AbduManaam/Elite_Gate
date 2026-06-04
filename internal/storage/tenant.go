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

// എല്ലാ repository-കൾക്കും common database functions share ചെയ്യാൻ ഉപയോഗിക്കുന്ന parent/base struct ആണ് BaseRepo.
type BaseRepo struct {
	db     *sql.DB
	logger zerolog.Logger
}

// setTenantSession എന്ന function PostgreSQL-ൽ current transaction-നു മാത്രം valid ആയ tenant information set ചെയ്യുന്നു.
// app.project_id = ഇപ്പോൾ ഏത് project (tenant) ആണ് access ചെയ്യുന്നത്
// app.current_user_id = ഇപ്പോൾ ഏത് user ആണ് request ചെയ്യുന്നത്

// TRUE എന്നത്:
// ഈ value transaction കഴിയുന്നത് വരെ മാത്രം നിലനിൽക്കണം

// 1.setTenantSession() project_id set ചെയ്യുന്നു
// 2.RLS ആ value ഉപയോഗിക്കുന്നു
// 3.User-ന് സ്വന്തം project data മാത്രം കാണാം

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

// withTenantTx Database query run ചെയ്യുന്നതിന് മുമ്പ് വേണ്ട എല്ലാ setup-ഉം ഇത് automatically ചെയ്യും.
// Get Tenant Info from Context
//         ↓
// Start Transaction  =  [ഇവിടെ current project id temporary ആയി set ചെയ്ത്, request കഴിഞ്ഞാൽ automatic remove ചെയ്യാൻ ആണ്. അതുവഴി tenant data mix ആകില്ല.Transaction ഇല്ലെങ്കിൽ: connection-ൽ value remain ചെയ്യാം.]
//         ↓
// Set app.project_id  = by calling setTenantSession() [ഇത് RLS policies ഉപയോഗിക്കും "app.project_id, app.current_user_id"]
//         ↓
// Run Query   =  Invokes the query callback function fn(tx).eg- routeRepo.Create(tx, route),routeRepo.List,Update
//         ↓
// Success? ── Yes → Commit
//         │
//         No
//         ↓
//      Rollback

// after this,the transaction ends, and PostgreSQL automatically removes those transaction-local settings.
// bcz of setTenantSession() uses:SELECT set_config('app.project_id', 'project-123', TRUE);
// .SELECT set_config('app.current_user_id', 'user-456', TRUE);
// the TRUE means:
// Set this value only for the current transaction.

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
