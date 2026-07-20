package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"

	"elitegate/helper"
)

// RequireGatewayToken authenticates a data-plane gateway container calling
// an /internal/v1/projects/:project_id/... endpoint. The expected token is
// DERIVED from masterSecret + the :project_id in the URL (same one-way HMAC
// pattern as helper.DeriveTenantJWTSecret, which is exactly what gateway
// containers are already given as JWT_SECRET). A gateway holding project
// A's token cannot use it against project B — the comparison target is
// computed from B's own project_id, which A's token was never derived from.
func RequireGatewayToken(masterSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID := c.Param("project_id")
		if projectID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "project_id required"})
			return
		}

		got := c.GetHeader("X-Gateway-Token")
		if got == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing gateway token"})
			return
		}

		want := helper.DeriveTenantJWTSecret(masterSecret, projectID)

		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid gateway token"})
			return
		}

		c.Next()
	}
}
