package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
		email,
		password_hash,
		google_id,
		avatar_url,
		auth_provider,
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
		&u.Email,
		&u.PasswordHash,
		&u.GoogleID,
		&u.AvatarURL,
		&u.AuthProvider,
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
		email,
		password_hash,
		google_id,
		avatar_url,
		auth_provider,
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
		&u.Email,
		&u.PasswordHash,
		&u.GoogleID,
		&u.AvatarURL,
		&u.AuthProvider,
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
	INSERT INTO admin_users (username, password_hash, email, is_super_admin, auth_provider)
	VALUES ($1, $2, $1 || '@elitegate.local', $3, 'password')
	ON CONFLICT (username) DO NOTHING
	RETURNING id, username, email, password_hash, google_id, avatar_url, auth_provider,
	          failed_login_attempts, locked_until, last_login_at, created_at
	`

	var u model.AdminUser
	err := r.db.QueryRowContext(ctx, q, username, passwordHash, isSuperAdmin).Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.PasswordHash,
		&u.GoogleID,
		&u.AvatarURL,
		&u.AuthProvider,
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
	username, email, passwordHash, companyName, slug, plan string,
) (*SignupResult, error) {

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("SignupTx: begin transaction: %w", err)
	}
	defer tx.Rollback() // no-op if committed; rolls back admin_user on project failure

	// ── Step 1: Insert admin_user (is_super_admin=FALSE — tenant, not operator) ──
	var user model.AdminUser
	const qUser = `
		INSERT INTO admin_users (username, password_hash, email, is_super_admin, auth_provider)
		VALUES ($1, $2, $3, FALSE, 'password')
		RETURNING id, username, email, password_hash, google_id, avatar_url, auth_provider,
		          failed_login_attempts, locked_until, last_login_at, created_at
	`
	err = tx.QueryRowContext(ctx, qUser, username, passwordHash, email).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.GoogleID, &user.AvatarURL, &user.AuthProvider,
		&user.FailedLoginAttempts, &user.LockedUntil,
		&user.LastLoginAt, &user.CreatedAt,
	)
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
		if isUniqueViolation(err) {
			suffix := user.ID
			if len(suffix) > 8 {
				suffix = suffix[:8]
			}
			retrySlug := slug + "-" + suffix
			err = tx.QueryRowContext(ctx, qProject, companyName, retrySlug, user.ID, plan).Scan(
				&project.ID, &project.IsActive, &project.CreatedAt, &project.UpdatedAt,
			)
			if err != nil {
				return nil, fmt.Errorf("SignupTx: insert project (retry slug %q): %w", retrySlug, err)
			}
			project.Slug = retrySlug
		} else {
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

// FindAdminUserByGoogleID — Case A: returning Google user.
func (r *AdminAuthRepo) FindAdminUserByGoogleID(ctx context.Context, googleID string) (*model.AdminUser, error) {
	const q = `
    SELECT id, username, email, password_hash, google_id, avatar_url, auth_provider,
           failed_login_attempts, locked_until, last_login_at, created_at
    FROM admin_users WHERE google_id = $1`
	var u model.AdminUser
	err := r.db.QueryRowContext(ctx, q, googleID).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.GoogleID, &u.AvatarURL, &u.AuthProvider,
		&u.FailedLoginAttempts, &u.LockedUntil, &u.LastLoginAt, &u.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return &u, err
}

// FindAdminUserByEmail — Case B lookup (email is CITEXT: case-insensitive already).
func (r *AdminAuthRepo) FindAdminUserByEmail(ctx context.Context, email string) (*model.AdminUser, error) {
	const q = `
    SELECT id, username, email, password_hash, google_id, avatar_url, auth_provider,
           failed_login_attempts, locked_until, last_login_at, created_at
    FROM admin_users WHERE email = $1`
	var u model.AdminUser
	err := r.db.QueryRowContext(ctx, q, email).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.GoogleID, &u.AvatarURL, &u.AuthProvider,
		&u.FailedLoginAttempts, &u.LockedUntil, &u.LastLoginAt, &u.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return &u, err
}

// LinkGoogleAccount — Case B: attach a google_id to an existing password account.
func (r *AdminAuthRepo) LinkGoogleAccount(ctx context.Context, userID, googleID, avatarURL string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE admin_users SET google_id = $2, avatar_url = COALESCE(NULLIF($3, ''), avatar_url)
         WHERE id = $1`,
		userID, googleID, avatarURL,
	)
	return err
}

// GoogleSignupTx — Case C: brand-new user via Google. Mirrors SignupTx exactly,
// but with no password and auth_provider='google'.
func (r *AdminAuthRepo) GoogleSignupTx(
	ctx context.Context, email, googleID, displayName, avatarURL, companyName, slug string,
) (*SignupResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("GoogleSignupTx: begin transaction: %w", err)
	}
	defer tx.Rollback()

	var user model.AdminUser
	const qUser = `
        INSERT INTO admin_users (username, email, google_id, avatar_url, auth_provider, is_super_admin)
        VALUES ($1, $2, $3, $4, 'google', FALSE)
        ON CONFLICT (email) DO NOTHING
        RETURNING id, username, email, password_hash, google_id, avatar_url, auth_provider,
                  failed_login_attempts, locked_until, last_login_at, created_at
    `
	// `username` has a UNIQUE NOT NULL constraint; derive one from the email
	// local-part since Google users never choose one explicitly.
	err = tx.QueryRowContext(ctx, qUser, usernameFromEmail(email), email, googleID, avatarURL).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.GoogleID,
		&user.AvatarURL, &user.AuthProvider, &user.FailedLoginAttempts,
		&user.LockedUntil, &user.LastLoginAt, &user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows // email or derived username collided — caller re-checks by email
	}
	if err != nil {
		return nil, fmt.Errorf("GoogleSignupTx: insert admin_user: %w", err)
	}

	var project model.Project
	project.Name, project.OwnerID = companyName, user.ID
	const qProject = `
        INSERT INTO projects (name, slug, description, owner_id, plan)
        VALUES ($1, $2, '', $3, 'free')
        RETURNING id, is_active, created_at, updated_at`
	err = tx.QueryRowContext(ctx, qProject, companyName, slug, user.ID).Scan(
		&project.ID, &project.IsActive, &project.CreatedAt, &project.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			retrySlug := slug + "-" + user.ID[:8]
			if err = tx.QueryRowContext(ctx, qProject, companyName, retrySlug, user.ID).Scan(
				&project.ID, &project.IsActive, &project.CreatedAt, &project.UpdatedAt,
			); err != nil {
				return nil, fmt.Errorf("GoogleSignupTx: insert project retry: %w", err)
			}
			project.Slug = retrySlug
		} else {
			return nil, fmt.Errorf("GoogleSignupTx: insert project: %w", err)
		}
	} else {
		project.Slug = slug
	}

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO project_members (project_id, admin_user_id, role) VALUES ($1, $2, 'owner')`,
		project.ID, user.ID,
	); err != nil {
		return nil, fmt.Errorf("GoogleSignupTx: insert project_member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("GoogleSignupTx: commit: %w", err)
	}
	return &SignupResult{User: user, Project: project}, nil
}

// usernameFromEmail derives a username from the email local-part.
// Example: "abdu.manam@gmail.com" -> "abdu.manam"
func usernameFromEmail(email string) string {
	email = strings.ToLower(email)

	parts := strings.SplitN(email, "@", 2)

	return parts[0]
}

var ErrInvalidPasswordResetToken = errors.New("invalid or expired password reset token")

func (r *AdminAuthRepo) ReplacePasswordResetTokenTx(
	ctx context.Context,
	adminUserID, tokenHash string,
	expiresAt time.Time,
) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("ReplacePasswordResetTokenTx begin: %w", err)
	}
	defer tx.Rollback()

	// Row-level lock serializes concurrent replacement requests for the same user
	var dummyID string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM admin_users WHERE id = $1 FOR UPDATE
	`, adminUserID).Scan(&dummyID)
	if err != nil {
		return "", fmt.Errorf("lock admin user row for reset token replacement: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE password_reset_tokens
		SET used_at = NOW()
		WHERE admin_user_id = $1 AND used_at IS NULL
	`, adminUserID)
	if err != nil {
		return "", fmt.Errorf("invalidate previous reset tokens: %w", err)
	}

	var newTokenID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO password_reset_tokens (admin_user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id
	`, adminUserID, tokenHash, expiresAt).Scan(&newTokenID)
	if err != nil {
		return "", fmt.Errorf("insert new reset token: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit reset token replace: %w", err)
	}

	return newTokenID, nil
}

func (r *AdminAuthRepo) InvalidatePasswordResetTokenByID(ctx context.Context, tokenID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE password_reset_tokens
		SET used_at = NOW()
		WHERE id = $1 AND used_at IS NULL
	`, tokenID)
	if err != nil {
		return fmt.Errorf("InvalidatePasswordResetTokenByID: %w", err)
	}
	return nil
}

func (r *AdminAuthRepo) FindValidPasswordResetToken(ctx context.Context, tokenHash string) (*model.PasswordResetToken, error) {
	var t model.PasswordResetToken
	err := r.db.QueryRowContext(ctx, `
		SELECT id, admin_user_id, token_hash, expires_at, used_at, created_at
		FROM password_reset_tokens
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW()
	`, tokenHash).Scan(&t.ID, &t.AdminUserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidPasswordResetToken
	}
	if err != nil {
		return nil, fmt.Errorf("FindValidPasswordResetToken: %w", err)
	}
	return &t, nil
}

func (r *AdminAuthRepo) ResetPasswordTx(ctx context.Context, resetTokenID, adminUserID, newPasswordHash string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ResetPasswordTx begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE password_reset_tokens
		SET used_at = NOW()
		WHERE id = $1 AND admin_user_id = $2 AND used_at IS NULL AND expires_at > NOW()
	`, resetTokenID, adminUserID)
	if err != nil {
		return fmt.Errorf("claim reset token exec: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("claim reset token rows affected: %w", err)
	}
	if rows == 0 {
		return ErrInvalidPasswordResetToken
	}

	resUser, err := tx.ExecContext(ctx, `
		UPDATE admin_users
		SET password_hash = $2, failed_login_attempts = 0, locked_until = NULL
		WHERE id = $1
	`, adminUserID, newPasswordHash)
	if err != nil {
		return fmt.Errorf("update user password exec: %w", err)
	}
	rowsUser, err := resUser.RowsAffected()
	if err != nil {
		return fmt.Errorf("update user password rows affected: %w", err)
	}
	if rowsUser == 0 {
		return fmt.Errorf("user not found for password reset")
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE admin_user_id = $1 AND revoked_at IS NULL
	`, adminUserID)
	if err != nil {
		return fmt.Errorf("revoke user refresh tokens: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ResetPasswordTx commit: %w", err)
	}

	return nil
}

func (r *AdminAuthRepo) DeleteExpiredPasswordResetTokens(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM password_reset_tokens
		WHERE expires_at < NOW() OR used_at IS NOT NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("DeleteExpiredPasswordResetTokens: %w", err)
	}
	return res.RowsAffected()
}
