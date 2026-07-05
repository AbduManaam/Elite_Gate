package model

import "time"

// Subscription holds billing, usage limits, and licensing details.
// Only accessible to the project owner.
type Subscription struct {
	Plan   string `json:"plan"`
	Status string `json:"status"` // e.g., "active", "trialing", "suspended"
}

// ProjectSummary is the aggregated dashboard payload for a single project.
type ProjectSummary struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Slug        string                `json:"slug"`
	Description string                `json:"description"`
	IsActive    bool                  `json:"is_active"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	Metrics     ProjectSummaryMetrics `json:"metrics"`

	// Subscription & Billing details — visible only to the Owner
	Plan         *string       `json:"plan,omitempty"`
	Subscription *Subscription `json:"subscription,omitempty"`
}

// ProjectSummaryMetrics holds every count shown on the dashboard.
// Visible to any project member (viewer, editor, owner).
type ProjectSummaryMetrics struct {
	TotalGateways     int `json:"total_gateways"`
	TotalRoutes       int `json:"total_routes"`
	EnabledRoutes     int `json:"enabled_routes"`
	TotalUpstreams     int `json:"total_upstreams"`
	EnabledUpstreams   int `json:"enabled_upstreams"`
	TotalPolicies      int `json:"total_policies"`
	TotalAPIKeys       int `json:"total_api_keys"`
	ActiveAPIKeys      int `json:"active_api_keys"`
	TotalMembers       int `json:"total_members"`
	TotalAuditLogs4d   int `json:"total_audit_logs_4d"` // capped 4-day window
}
