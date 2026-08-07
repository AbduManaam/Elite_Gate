package model

import "time"

const (
	JWTAlgorithmHS256 = "HS256"
)

// ProjectJWTConfig represents the JWT verification configuration
// owned by a single EliteGate project.
//
// The actual customer JWT secret is never stored in this struct
// or PostgreSQL. SecretARN points to the secret stored securely
// in AWS Secrets Manager.
type ProjectJWTConfig struct {
	ProjectID string `json:"project_id"`

	Enabled   bool   `json:"enabled"`
	Algorithm string `json:"algorithm"`

	// Internal infrastructure fields.
	// Never expose these through normal Admin API responses.
	SecretARN       string `json:"-"`
	SecretVersionID string `json:"-"`

	// Incremented whenever JWT verification configuration changes.
	ConfigVersion int64 `json:"config_version"`

	// Optional JWT validation constraints.
	Issuer    *string  `json:"issuer,omitempty"`
	Audiences []string `json:"audiences"`

	// Names of claims EliteGate should read from customer JWTs.
	SubjectClaim string `json:"subject_claim"`
	RoleClaim    string `json:"role_claim"`
	ScopesClaim  string `json:"scopes_claim"`

	ClockSkewSeconds int `json:"clock_skew_seconds"`

	CreatedBy *string `json:"-"`
	UpdatedBy *string `json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
