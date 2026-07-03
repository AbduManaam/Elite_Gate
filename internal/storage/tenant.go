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
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var ErrNotFound = errors.New("resource not found")
var ErrForbidden = errors.New("access denied")
var ErrAlreadyMember = errors.New("user is already a member of this project")
var ErrLastOwner = errors.New("cannot remove or demote the last owner of the project")
var ErrIsProjectOwner = errors.New("cannot remove the project owner; transfer ownership first")

// A struct carrying the identity of the current project (ProjectID), the active user (UserID), and their permission role (UserRole).
type TenantContext struct {
	ProjectID uuid.UUID
	UserID    uuid.UUID
	UserRole  string
}

type ProjectMember struct {
	ProjectID   uuid.UUID `json:"project_id"`
	AdminUserID uuid.UUID `json:"admin_user_id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joined_at"`
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

/*
setTenantSession sets tenant information in PostgreSQL
for the current transaction only.

app.project_id      = which project (tenant) is being accessed
app.current_user_id = which user is making the request

TRUE means the value exists only for the current transaction
and is automatically removed when the transaction ends.

Flow:
1. setTenantSession() sets app.project_id
2. RLS policies use that value
3. The user can access only data from their own project
*/
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
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
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
		return "", ErrForbidden
	}
	if err != nil {
		return "", fmt.Errorf("ValidateMembership: %w", err)
	}

	r.logger.Debug().
		Str("project_id", projectID.String()).
		Str("user_id", userID.String()).
		Str("role", role).
		Msg("membership validation successful")
	return role, nil
}

func (r *MembershipRepo) AddMember(ctx context.Context, projectID, userID uuid.UUID, role string, inviterID uuid.UUID) error {
	// Check if already a member
	var exists int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM project_members WHERE project_id = $1 AND admin_user_id = $2`, projectID, userID).Scan(&exists)
	if err == nil {
		return ErrAlreadyMember
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check membership: %w", err)
	}

	// Insert member
	const q = `
		INSERT INTO project_members (project_id, admin_user_id, role, invited_by, joined_at)
		VALUES ($1, $2, $3, $4, NOW())
	`
	_, err = r.db.ExecContext(ctx, q, projectID, userID, role, inviterID)
	if err != nil {
		return fmt.Errorf("insert project member: %w", err)
	}
	return nil
}

func (r *MembershipRepo) UpdateRole(ctx context.Context, projectID, memberID uuid.UUID, role string) error {
	var currentRole string
	err := r.db.QueryRowContext(ctx, `SELECT role FROM project_members WHERE project_id = $1 AND admin_user_id = $2`, projectID, memberID).Scan(&currentRole)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get member role: %w", err)
	}

	if currentRole == role {
		return nil
	}

	if currentRole == "owner" && role != "owner" {
		var ownerCount int
		err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_members WHERE project_id = $1 AND role = 'owner'`, projectID).Scan(&ownerCount)
		if err != nil {
			return fmt.Errorf("count project owners: %w", err)
		}
		if ownerCount <= 1 {
			return ErrLastOwner
		}
	}

	const q = `
		UPDATE project_members
		SET    role = $3
		WHERE  project_id = $1 AND admin_user_id = $2
	`
	res, err := r.db.ExecContext(ctx, q, projectID, memberID, role)
	if err != nil {
		return fmt.Errorf("update member role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MembershipRepo) RemoveMember(ctx context.Context, projectID, memberID uuid.UUID) error {
	var ownerID uuid.UUID
	err := r.db.QueryRowContext(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get project owner: %w", err)
	}
	if ownerID == memberID {
		return ErrIsProjectOwner
	}

	var currentRole string
	err = r.db.QueryRowContext(ctx, `SELECT role FROM project_members WHERE project_id = $1 AND admin_user_id = $2`, projectID, memberID).Scan(&currentRole)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get member role: %w", err)
	}

	if currentRole == "owner" {
		var ownerCount int
		err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_members WHERE project_id = $1 AND role = 'owner'`, projectID).Scan(&ownerCount)
		if err != nil {
			return fmt.Errorf("count project owners: %w", err)
		}
		if ownerCount <= 1 {
			return ErrLastOwner
		}
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM project_members WHERE project_id = $1 AND admin_user_id = $2`, projectID, memberID)
	if err != nil {
		return fmt.Errorf("delete member: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MembershipRepo) ListMembers(ctx context.Context, projectID uuid.UUID) ([]ProjectMember, error) {
	const q = `
		SELECT pm.project_id, pm.admin_user_id, au.username, au.email, pm.role, pm.joined_at
		FROM   project_members pm
		JOIN   admin_users      au ON au.id = pm.admin_user_id
		WHERE  pm.project_id = $1
		  AND  au.deleted_at IS NULL
		ORDER BY pm.joined_at ASC
	`
	rows, err := r.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, fmt.Errorf("list members query: %w", err)
	}
	defer rows.Close()

	var members []ProjectMember
	for rows.Next() {
		var m ProjectMember
		if err := rows.Scan(&m.ProjectID, &m.AdminUserID, &m.Username, &m.Email, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}

type MemberLookupResult struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
	Email    string    `json:"email"`
}

func (r *MembershipRepo) FindUserByEmail(ctx context.Context, email string) (MemberLookupResult, error) {
	const q = `
		SELECT id, username, email
		FROM   admin_users
		WHERE  email = $1
		  AND  is_active  = TRUE
		  AND  deleted_at IS NULL
	`
	var res MemberLookupResult
	err := r.db.QueryRowContext(ctx, q, email).Scan(&res.ID, &res.Username, &res.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MemberLookupResult{}, sql.ErrNoRows
		}
		return MemberLookupResult{}, fmt.Errorf("FindUserByEmail: %w", err)
	}
	return res, nil
}
