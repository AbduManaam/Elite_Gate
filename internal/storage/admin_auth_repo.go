package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"elitegate/internal/model"
)

// Repository object holding DB connection

type AdminAuthRepo struct {
	db *sql.DB
}

// Constructor
func NewAdminAuthRepo(db *sql.DB) *AdminAuthRepo {
	return &AdminAuthRepo{db: db}
}

// Find admin user by username(Used during login)
func (r *AdminAuthRepo) FindAdminUserByUsername(
	ctx context.Context,
	username string,
) (*model.AdminUser, error) {

	const q = `
	SELECT
		id,
		username,
		password_hash,
		failed_login_attempts,
		locked_until,
		last_login_at,
		created_at
	FROM admin_users
	WHERE username = $1
	`

	var u model.AdminUser

	err := r.db.QueryRowContext(ctx, q, username).Scan(
		&u.ID,
		&u.Username,
		&u.PasswordHash,
		&u.FailedLoginAttempts,
		&u.LockedUntil,
		&u.LastLoginAt,
		&u.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}

	if err != nil {
		return nil, err
	}

	return &u, nil
}

func (r *AdminAuthRepo) FindAdminUserByID(
	ctx context.Context,
	id string,
) (*model.AdminUser, error) {
	const q = `
	SELECT
		id,
		username,
		password_hash,
		failed_login_attempts,
		locked_until,
		last_login_at,
		created_at
	FROM admin_users
	WHERE id = $1
	`

	var u model.AdminUser

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&u.ID,
		&u.Username,
		&u.PasswordHash,
		&u.FailedLoginAttempts,
		&u.LockedUntil,
		&u.LastLoginAt,
		&u.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &u, nil
}

// Store refresh token in DB

func (r *AdminAuthRepo) CreateRefreshToken(
	ctx context.Context,
	userID string,
	tokenHash string,
	expiresAt time.Time,
	ip string,
	ua string,
) error {

	const q = `
	INSERT INTO refresh_tokens
	(admin_user_id, token_hash, expires_at, ip_address, user_agent)
	VALUES ($1, $2, $3, $4, $5)
	`

	_, err := r.db.ExecContext(
		ctx,
		q,
		userID,
		tokenHash,
		expiresAt,
		nullText(ip),
		nullText(ua),
	)

	return err
}

// Find refresh token
// Used during token refresh

func (r *AdminAuthRepo) FindRefreshToken(
	ctx context.Context,
	tokenHash string,
) (*model.RefreshToken, error) {

	const q = `
	SELECT
		id,
		admin_user_id,
		token_hash,
		expires_at,
		revoked_at,
		ip_address,
		user_agent,
		created_at
	FROM refresh_tokens
	WHERE token_hash = $1
	`

	var t model.RefreshToken

	err := r.db.QueryRowContext(ctx, q, tokenHash).Scan(
		&t.ID,
		&t.AdminUserID,
		&t.TokenHash,
		&t.ExpiresAt,
		&t.RevokedAt,
		&t.IPAddress,
		&t.UserAgent,
		&t.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}

	if err != nil {
		return nil, err
	}

	return &t, nil
}

// Revoke single refresh token
// Used during logout

func (r *AdminAuthRepo) RevokeRefreshToken(
	ctx context.Context,
	tokenHash string,
) error {

	_, err := r.db.ExecContext(
		ctx,
		`
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE token_hash = $1
		AND revoked_at IS NULL
		`,
		tokenHash,
	)

	return err
}

// Rotate refresh token securely
// Old token revoked
// New token inserted

func (r *AdminAuthRepo) RotateRefreshToken(
	ctx context.Context,
	oldHash string,
	newHash string,
	userID string,
	newExpiresAt time.Time,
	ip string,
	ua string,
) error {

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	// Revoke old token

	res, err := tx.ExecContext(
		ctx,
		`
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE token_hash = $1
		AND revoked_at IS NULL
		`,
		oldHash,
	)

	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if affected != 1 {
		return sql.ErrNoRows
	}

	// Insert new token

	_, err = tx.ExecContext(
		ctx,
		`
		INSERT INTO refresh_tokens
		(admin_user_id, token_hash, expires_at, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5)
		`,
		userID,
		newHash,
		newExpiresAt,
		nullText(ip),
		nullText(ua),
	)

	if err != nil {
		return err
	}

	return tx.Commit()
}

// Delete expired / old revoked tokens

func (r *AdminAuthRepo) PruneExpiredTokens(
	ctx context.Context,
) error {

	_, err := r.db.ExecContext(
		ctx,
		`
		DELETE FROM refresh_tokens
		WHERE expires_at < now()
		OR revoked_at < now() - interval '7 days'
		`,
	)

	return err
}

// Reset failed login attempts after successful login

func (r *AdminAuthRepo) UpdateAdminLoginSuccess(
	ctx context.Context,
	userID string,
) error {

	_, err := r.db.ExecContext(
		ctx,
		`
		UPDATE admin_users
		SET
			failed_login_attempts = 0,
			locked_until = NULL,
			last_login_at = now()
		WHERE id = $1
		`,
		userID,
	)

	return err
}

// Increase failed login attempts

func (r *AdminAuthRepo) IncrementAdminLoginFailure(
	ctx context.Context,
	username string,
) error {

	_, err := r.db.ExecContext(
		ctx,
		`
		UPDATE admin_users
		SET failed_login_attempts =
			failed_login_attempts + 1
		WHERE username = $1
		`,
		username,
	)

	return err
}

// Lock account until specific time

func (r *AdminAuthRepo) LockAdminUser(
	ctx context.Context,
	username string,
	until time.Time,
) error {

	_, err := r.db.ExecContext(
		ctx,
		`
		UPDATE admin_users
		SET locked_until = $2
		WHERE username = $1
		`,
		username,
		until,
	)

	return err
}

// AdminUserCount returns the total number of admin users in the system.
// Used by the register endpoint to detect first-run (bootstrap) mode.

func (r *AdminAuthRepo) AdminUserCount(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&count)
	return count, err
}

// CreateAdminUser inserts a new admin user with a pre-hashed password.
// Set isSuperAdmin=true ONLY for the one-time bootstrap operator account.
// All tenant accounts created via POST /admin/signup use SignupTx instead
// (which is atomic). This method is for bootstrap + operator-support only.
//
// Returns sql.ErrNoRows if the username is already taken (ON CONFLICT DO NOTHING).
func (r *AdminAuthRepo) CreateAdminUser(
	ctx context.Context,
	username string,
	passwordHash string,
	isSuperAdmin bool,
) (*model.AdminUser, error) {

	const q = `
	INSERT INTO admin_users (username, password_hash, email, is_super_admin)
	VALUES ($1, $2, $1 || '@elitegate.local', $3)
	ON CONFLICT (username) DO NOTHING
	RETURNING id, username, password_hash, failed_login_attempts,
	          locked_until, last_login_at, created_at
	`

	var u model.AdminUser
	err := r.db.QueryRowContext(ctx, q, username, passwordHash, isSuperAdmin).Scan(
		&u.ID,
		&u.Username,
		&u.PasswordHash,
		&u.FailedLoginAttempts,
		&u.LockedUntil,
		&u.LastLoginAt,
		&u.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows // username conflict
	}
	if err != nil {
		return nil, err
	}

	return &u, nil
}

// Convert string → sql.NullString
// Used for nullable DB fields
func nullText(v string) sql.NullString {
	return sql.NullString{
		String: v,
		Valid:  v != "",
	}
}

// IsSuperAdmin returns true if the given admin user has the is_super_admin flag set.
// Used by SuperAdminOnly middleware to gate platform-operator-only endpoints.
// This is intentionally a simple boolean lookup — future operator features
// (tenant suspension, impersonation, secret rotation) reuse this same flag.
func (r *AdminAuthRepo) IsSuperAdmin(ctx context.Context, userID string) (bool, error) {
	var flag bool
	err := r.db.QueryRowContext(ctx,
		`SELECT is_super_admin FROM admin_users WHERE id = $1`,
		userID,
	).Scan(&flag)
	if errors.Is(err, sql.ErrNoRows) {
		// Not an error condition — caller (SuperAdminOnly middleware) treats
		// "user not found" the same as "not a super-admin": deny access.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("IsSuperAdmin query for user %s: %w", userID, err)
	}
	return flag, nil
}

// SignupResult holds the admin user and project created atomically by SignupTx.
type SignupResult struct {
	User    model.AdminUser
	Project model.Project
}

// SignupTx atomically creates an admin_user, a project, and the owner
// project_members row in a single database transaction.
//
// If project creation fails for any reason, the admin_user insert is rolled
// back — preventing orphaned admin accounts with no associated project.
//
// Returns sql.ErrNoRows if the username is already taken (ON CONFLICT DO NOTHING).
func (r *AdminAuthRepo) SignupTx(
	ctx context.Context,
	username, passwordHash, companyName, slug, plan string,
) (*SignupResult, error) {

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("SignupTx: begin transaction: %w", err)
	}
	defer tx.Rollback() // no-op if committed; rolls back admin_user on project failure

	// ── Step 1: Insert admin_user (is_super_admin=FALSE — tenant, not operator) ──
	var user model.AdminUser
	const qUser = `
		INSERT INTO admin_users (username, password_hash, email, is_super_admin)
		VALUES ($1, $2, $1 || '@elitegate.local', FALSE)
		ON CONFLICT (username) DO NOTHING
		RETURNING id, username, password_hash, failed_login_attempts,
		          locked_until, last_login_at, created_at
	`
	err = tx.QueryRowContext(ctx, qUser, username, passwordHash).Scan(
		&user.ID, &user.Username, &user.PasswordHash,
		&user.FailedLoginAttempts, &user.LockedUntil,
		&user.LastLoginAt, &user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows // username already taken
	}
	if err != nil {
		return nil, fmt.Errorf("SignupTx: insert admin_user: %w", err)
	}

	// ── Step 2: Insert project ────────────────────────────────────────────────
	var project model.Project
	project.Name = companyName
	project.OwnerID = user.ID
	project.Plan = plan

	const qProject = `
		INSERT INTO projects (name, slug, description, owner_id, plan)
		VALUES ($1, $2, '', $3, $4)
		RETURNING id, is_active, created_at, updated_at
	`
	err = tx.QueryRowContext(ctx, qProject, companyName, slug, user.ID, plan).Scan(
		&project.ID, &project.IsActive, &project.CreatedAt, &project.UpdatedAt,
	)
	if err != nil {
		var pqErr interface{ Code() string }
		if isUniqueViolation(err) {
			// Slug collision — retry once with a user-ID suffix.
			retrySlug := slug + "-" + user.ID[:8]
			err = tx.QueryRowContext(ctx, qProject, companyName, retrySlug, user.ID, plan).Scan(
				&project.ID, &project.IsActive, &project.CreatedAt, &project.UpdatedAt,
			)
			if err != nil {
				// Rollback is automatic via defer — no orphaned user.
				return nil, fmt.Errorf("SignupTx: insert project (retry slug %q): %w", retrySlug, err)
			}
			project.Slug = retrySlug
		} else {
			_ = pqErr // silence unused variable
			// Rollback is automatic via defer — no orphaned user.
			return nil, fmt.Errorf("SignupTx: insert project: %w", err)
		}
	} else {
		project.Slug = slug
	}

	// ── Step 3: Insert owner membership ──────────────────────────────────────
	const qMember = `
		INSERT INTO project_members (project_id, admin_user_id, role)
		VALUES ($1, $2, 'owner')
	`
	if _, err = tx.ExecContext(ctx, qMember, project.ID, user.ID); err != nil {
		return nil, fmt.Errorf("SignupTx: insert project_member: %w", err)
	}

	// ── Commit — all three rows land atomically ───────────────────────────────
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("SignupTx: commit: %w", err)
	}

	return &SignupResult{User: user, Project: project}, nil
}
