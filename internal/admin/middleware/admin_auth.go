package middleware
// 1. Read Authorization header
// 2. Extract JWT token
// 3. Validate token
// 4. Check admin role
// 5. Store user info in request context
// 6. Allow request to continue

import (
	"net/http"
	"strings"

	authpkg "elitegate/internal/auth"

	"github.com/gin-gonic/gin"
)

const AdminUserIDKey = "admin_user_id"
const AdminUsernameKey = "admin_username"

func AdminAuth(tokens *authpkg.AdminTokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {

		header := c.GetHeader("Authorization")

		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(
				http.StatusUnauthorized,
				gin.H{
					"error": "missing bearer token",
				},
			)
			return
		}

		raw := strings.TrimSpace(
			strings.TrimPrefix(header, "Bearer "),
		)

		claims, err := tokens.ValidateAdminAccessToken(raw)
		if err != nil {
			c.AbortWithStatusJSON(
				http.StatusUnauthorized,
				gin.H{
					"error": "invalid token",
				},
			)
			return
		}

		if claims.Role != authpkg.AdminRole {
			c.AbortWithStatusJSON(
				http.StatusForbidden,
				gin.H{
					"error": "forbidden",
				},
			)
			return
		}

		c.Set(AdminUserIDKey, claims.Subject)
		c.Set(AdminUsernameKey, claims.Username)

		c.Next()
	}
}