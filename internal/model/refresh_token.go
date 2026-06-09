package model

import (
	"database/sql"
	"time"
)

// RefreshToken represents a row in the refresh_tokens table.
type RefreshToken struct {
	ID          string         `json:"id"`
	AdminUserID string         `json:"admin_user_id"`
	TokenHash   string         `json:"token_hash"`
	ExpiresAt   time.Time      `json:"expires_at"`
	RevokedAt   sql.NullTime   `json:"revoked_at"`
	IPAddress   sql.NullString `json:"ip_address"`
	UserAgent   sql.NullString `json:"user_agent"`
	CreatedAt   time.Time      `json:"created_at"`
}
