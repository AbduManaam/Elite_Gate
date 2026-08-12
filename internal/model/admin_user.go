package model

import (
	"database/sql"
	"time"
)

// AdminUser represents a row in the admin_users table.
type AdminUser struct {
	ID                  string         `json:"id"`
	Username            string         `json:"username"`
	Email               string         `json:"email"`
	PasswordHash        sql.NullString `json:"-"`
	GoogleID            sql.NullString `json:"google_id"`
	AvatarURL           sql.NullString `json:"avatar_url"`
	AuthProvider        string         `json:"auth_provider"`
	EmailVerified       bool           `json:"email_verified"`
	FailedLoginAttempts int            `json:"failed_login_attempts"`
	LockedUntil         sql.NullTime   `json:"locked_until"`
	LastLoginAt         sql.NullTime   `json:"last_login_at"`
	CreatedAt           time.Time      `json:"created_at"`
}
