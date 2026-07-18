package model

import "time"

type AuditLog struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	AdminUserID string    `json:"admin_user_id"`
	Actor       string    `json:"actor"` // joined from admin_users.username
	Action      string    `json:"action"`
	EntityType  string    `json:"entity_type"`
	EntityID    string    `json:"entity_id"`
	EntityLabel string    `json:"entity_label"`
	Changes     string    `json:"changes"`
	IPAddress   string    `json:"ip_address"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type AuditLogFilter struct {
	Limit    int
	Offset   int
	Actor    string // matches admin_users.username, partial match
	Action   string // exact match against audit_action enum value
	DateFrom *time.Time
	DateTo   *time.Time
}

type AuditLogPage struct {
	Logs  []AuditLog `json:"logs"`
	Total int        `json:"total"`
}
