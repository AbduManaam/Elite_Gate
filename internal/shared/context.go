package shared

type ContextKey string

const (
	ContextKeyClientID     ContextKey = "clientID"
	ContextKeyRole         ContextKey = "role"
	ContextKeyRequestID    ContextKey = "requestID"
	ContextKeyRoute        ContextKey = "route"
	ContextKeyAuthInfo     ContextKey = "authInfo"
	ContextKeyScopes       ContextKey = "scopes"
	ContextKeyCustomDomain ContextKey = "customDomain"
)
