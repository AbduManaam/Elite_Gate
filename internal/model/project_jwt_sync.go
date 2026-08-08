package model

// ProjectJWTConfigSync contains only the JWT verification metadata a
// project gateway needs.
//
// The raw customer signing secret is NEVER included in this structure.
// SecretARN and SecretVersionID point to the value stored in AWS
// Secrets Manager and are used by the gateway only when its verifier
// needs to be built or refreshed.
type ProjectJWTConfigSync struct {
	Enabled bool `json:"enabled"`

	Algorithm string `json:"algorithm"`

	// Internal AWS reference. This is not the secret value.
	//
	// These fields are populated only while JWT authentication is enabled.
	SecretARN       string `json:"secret_arn,omitempty"`
	SecretVersionID string `json:"secret_version_id,omitempty"`

	// Incremented whenever the project's JWT configuration changes.
	ConfigVersion int64 `json:"config_version"`

	Issuer *string `json:"issuer,omitempty"`

	Audiences []string `json:"audiences"`

	SubjectClaim string `json:"subject_claim"`
	RoleClaim    string `json:"role_claim"`
	ScopesClaim  string `json:"scopes_claim"`

	ClockSkewSeconds int `json:"clock_skew_seconds"`
}
