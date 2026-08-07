package model

type ProjectJWTConfigSync struct {
	Enabled   bool   `json:"enabled"`
	Algorithm string `json:"algorithm"`

	SecretARN       string `json:"secret_arn,omitempty"`
	SecretVersionID string `json:"secret_version_id,omitempty"`

	ConfigVersion int64 `json:"config_version"`

	Issuer    *string  `json:"issuer,omitempty"`
	Audiences []string `json:"audiences"`

	SubjectClaim string `json:"subject_claim"`
	RoleClaim    string `json:"role_claim"`
	ScopesClaim  string `json:"scopes_claim"`

	ClockSkewSeconds int `json:"clock_skew_seconds"`
}
