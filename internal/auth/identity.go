package auth

// Identity is the normalized authenticated identity used by the gateway.
//
// Different authentication mechanisms can produce this structure:
//   - customer JWT
//   - legacy EliteGate JWT
//   - future OIDC/JWKS validators
//
// Middleware should depend on Identity instead of token-specific claims.
type Identity struct {
	ClientID string
	Role     string
	Scopes   []string
}
