package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)


// admin_users table row

type AdminUser struct {
	ID                   string
	Username             string
	PasswordHash         string
	FailedLoginAttempts  int
	LockedUntil          sql.NullTime
	LastLoginAt          sql.NullTime
	CreatedAt            time.Time
}


// refresh_tokens table row

type RefreshToken struct {
	ID          string
	AdminUserID string
	TokenHash   string
	ExpiresAt   time.Time
	RevokedAt   sql.NullTime
	IPAddress   sql.NullString
	UserAgent   sql.NullString
	CreatedAt   time.Time
}


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
) (*AdminUser, error) {

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

	var u AdminUser

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
) (*AdminUser, error) {
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

	var u AdminUser

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
) (*RefreshToken, error) {

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

	var t RefreshToken

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
// Returns sql.ErrNoRows if the username is already taken (ON CONFLICT DO NOTHING).

func (r *AdminAuthRepo) CreateAdminUser(
	ctx context.Context,
	username string,
	passwordHash string,
) (*AdminUser, error) {

	const q = `
	INSERT INTO admin_users (username, password_hash)
	VALUES ($1, $2)
	ON CONFLICT (username) DO NOTHING
	RETURNING id, username, password_hash, failed_login_attempts,
	          locked_until, last_login_at, created_at
	`

	var u AdminUser
	err := r.db.QueryRowContext(ctx, q, username, passwordHash).Scan(
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