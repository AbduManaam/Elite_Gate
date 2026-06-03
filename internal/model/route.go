package model

import "time"

// Policy holds reusable access-control rules shared across many routes.
type Policy struct {
	ID           string
	ProjectID    string    `json:"project_id"`
	Name         string    `json:"name"`
	AuthRequired bool      `json:"auth_required"`
	RateLimitRPM int       `json:"rate_limit_rpm"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Route represents a single gateway routing rule.
type Route struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	UpstreamID   *string   `json:"upstream_id"`
	UpstreamURL  string    `json:"upstream_url"`  // read-only: joined from upstreams
	Methods      []string  `json:"methods"`       // stored directly as a methods array column
	Protocol     string    `json:"protocol"`       // read-only: joined from upstreams
	MatchType    string    `json:"match_type"`
	Enabled      bool      `json:"enabled"`
	PolicyID     *string   `json:"policy_id"`
	AuthRequired bool      `json:"auth_required"` // read-only: joined from policies
	RateLimitRPM int       `json:"rate_limit_rpm"` // read-only: joined from policies
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Upstream represents a backend service the gateway can route traffic to.
type Upstream struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Name       string    `json:"name"`
	TargetURL  string    `json:"target_url"`
	Protocol   string    `json:"protocol"`
	HealthPath string    `json:"health_path"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
