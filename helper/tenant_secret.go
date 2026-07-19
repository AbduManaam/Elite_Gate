package helper

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// DeriveTenantJWTSecret derives a per-project secret from the platform's
// master JWT secret via HMAC-SHA256. This is a one-way derivation: leaking
// a derived secret does not expose the master secret or any other
// project's derived secret, and tokens signed with a derived secret will
// correctly fail AdminTokenManager.ValidateAdminAccessToken, which checks
// against the raw master secret.
func DeriveTenantJWTSecret(masterSecret, projectID string) string {
	mac := hmac.New(sha256.New, []byte(masterSecret))
	mac.Write([]byte("tenant_jwt_secret:v1:" + projectID))
	return hex.EncodeToString(mac.Sum(nil))
}
