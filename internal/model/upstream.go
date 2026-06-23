package model

import "time"

// Upstream represents a backend service the gateway can route traffic to.
type Upstream struct {
	ID         string           `json:"id"`
	ProjectID  string           `json:"project_id"`
	Name       string           `json:"name"`
	TargetURL  string           `json:"target_url"`
	Protocol   string           `json:"protocol"`
	HealthPath string           `json:"health_path"`
	Enabled    bool             `json:"enabled"`
	LBStrategy string           `json:"lb_strategy"`
	Targets    []UpstreamTarget `json:"targets,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

// UpstreamTarget represents a backend instance in an upstream pool
type UpstreamTarget struct {
	ID         string    `json:"id"`
	UpstreamID string    `json:"upstream_id"`
	TargetURL  string    `json:"target_url"`
	Weight     int       `json:"weight"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
