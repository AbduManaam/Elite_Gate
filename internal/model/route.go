package model

import "time"

// Policy holds reusable access-control rules shared across many routes.
// Extracted from routes to eliminate repeated auth_required/rate_limit_rpm columns.
type Policy struct {
	ID           string
	Name         string
	AuthRequired bool
	RateLimitRPM int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Route represents a single gateway routing rule.
// UpstreamURL and Protocol are populated via JOIN from upstreams.target_url / upstreams.protocol.
// Methods is populated via JOIN from the route_methods table.
// AuthRequired and RateLimitRPM are populated via JOIN from the policies table.
type Route struct {
	ID           string
	Path         string
	UpstreamID   *string
	UpstreamURL  string   // read-only: joined from upstreams.target_url
	Methods      []string // read-only: joined from route_methods
	Protocol     string   // read-only: joined from upstreams.protocol
	MatchType    string
	Enabled      bool
	PolicyID     *string
	AuthRequired bool // read-only: joined from policies.auth_required
	RateLimitRPM int  // read-only: joined from policies.rate_limit_rpm
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Upstream represents a backend service the gateway can route traffic to.
type Upstream struct {
	ID         string
	Name       string
	TargetURL  string
	Protocol   string
	HealthPath string
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
