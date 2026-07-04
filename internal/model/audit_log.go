package model

import "time"

type AuditLog struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	AdminUserID string    `json:"admin_user_id"`
	Action      string    `json:"action"`
	EntityType  string    `json:"entity_type"`
	EntityID    string    `json:"entity_id"`
	Changes     string    `json:"changes"`
	CreatedAt   time.Time `json:"created_at"`
}

// AuditLogFilter holds optional filters for listing audit logs.
type AuditLogFilter struct {
	Limit  int // max rows to return; defaults to 100 if zero
	Offset int // pagination offset
}
