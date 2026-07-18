package model

import "time"

// Route represents a single gateway routing rule.
type Route struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	Name           string    `json:"name"`
	Path           string    `json:"path"`
	UpstreamID     *string   `json:"upstream_id"`
	UpstreamURL    string    `json:"upstream_url"` // read-only: joined from upstreams
	Methods        []string  `json:"methods"`      // stored directly as a methods array column
	Protocol       string    `json:"protocol"`     // read-only: joined from upstreams
	MatchType      string    `json:"match_type"`
	Enabled        bool      `json:"enabled"`
	PolicyID       *string   `json:"policy_id"`
	AuthRequired   bool      `json:"auth_required"`  // read-only: joined from policies
	RateLimitRPM   int       `json:"rate_limit_rpm"` // read-only: joined from policies
	AllowedOrigins []string  `json:"allowed_origins"`
	AllowedRoles   []string  `json:"allowed_roles"`
	AllowedScopes  []string  `json:"allowed_scopes"`
	IPAllowlist    []string  `json:"ip_allowlist"`
	IPBlocklist    []string  `json:"ip_blocklist"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
