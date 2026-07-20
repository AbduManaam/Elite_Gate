package model

import (
	"database/sql"
	"time"
)

type PasswordResetToken struct {
	ID          string
	AdminUserID string
	TokenHash   string
	ExpiresAt   time.Time
	UsedAt      sql.NullTime
	CreatedAt   time.Time
}
