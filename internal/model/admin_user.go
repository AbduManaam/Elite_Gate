package model

import (
	"database/sql"
	"time"
)

// AdminUser represents a row in the admin_users table.
type AdminUser struct {
	ID                  string       `json:"id"`
	Username            string       `json:"username"`
	PasswordHash        string       `json:"-"`
	FailedLoginAttempts int          `json:"failed_login_attempts"`
	LockedUntil         sql.NullTime `json:"locked_until"`
	LastLoginAt         sql.NullTime `json:"last_login_at"`
	CreatedAt           time.Time    `json:"created_at"`
}
