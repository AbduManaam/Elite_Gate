package middleware

import (
	"net/http"

	"elitegate/helper"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// SuperAdminOnly blocks any authenticated admin who does NOT have is_super_admin=TRUE.
//
// This middleware is intentionally generic — it gates on a single DB flag so that
// all future platform-operator features (tenant suspension, secret rotation,
// impersonation, system-wide audit) can attach here without a schema change.
//
// Must be placed AFTER AdminAuth middleware (which sets admin_user_id in context).
//
// Usage:
//
//	admins.POST("", middleware.SuperAdminOnly(authRepo, logger), authHandler.Register)
func SuperAdminOnly(repo *storage.AdminAuthRepo, logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDVal, exists := c.Get(AdminUserIDKey)
		if !exists {
			// AdminAuth should have caught this — guard defensively.
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}
		userID := userIDVal.(string)

		ok, err := repo.IsSuperAdmin(c.Request.Context(), userID)
		if err != nil {
			helper.RespondInternalError(c, logger.With().Str("user_id", userID).Logger(), err, "super-admin check db error")
			return
		}
		if !ok {
			logger.Warn().
				Str("user_id", userID).
				Str("path", c.FullPath()).
				Msg("non-super-admin attempted platform-operator endpoint")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "super-admin access required"})
			return
		}

		c.Next()
	}
}
