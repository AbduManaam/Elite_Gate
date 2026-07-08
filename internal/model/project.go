package model

import "time"

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"` // unique ID (like acme-corp) used to identify a project in web URLs and routing paths
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	IsActive    bool      `json:"is_active"`
	Plan                    string    `json:"plan"` //The subscription tier or service level of the project.
	DashboardAllowedOrigins []string  `json:"dashboard_allowed_origins"`
	Role                    string    `json:"role,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}
