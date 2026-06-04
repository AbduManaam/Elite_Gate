package middleware

import (
	"errors"
	"net/http"

	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleOwner  Role = "owner"
)

func ProjectScope(membershipRepo *storage.MembershipRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectIDStr := c.Param("projectId")
		if projectIDStr == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "project ID required"})
			return
		}

		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid project ID format"})
			return
		}

		adminUserIDStr, exists := c.Get(AdminUserIDKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}

		str, ok := adminUserIDStr.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			return
		}

		userID, err := uuid.Parse(str)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user ID format"})
			return
		}

		role, err := membershipRepo.ValidateMembership(c.Request.Context(), projectID, userID)
		if err != nil {
			if errors.Is(err, storage.ErrForbidden) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "access denied to project"})
				return
			}
			// DB error — log but don't expose
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		tc := storage.TenantContext{
			ProjectID: projectID,
			UserID:    userID,
			UserRole:  role,
		}
		c.Set("tenant_ctx", tc)
		ctx := storage.WithTenantContext(c.Request.Context(), tc)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func RBAC(minRole Role) gin.HandlerFunc {
	roleWeights := map[string]int{
		"viewer": 0,
		"editor": 1,
		"owner":  2,
	}

	minWeight, ok := roleWeights[string(minRole)]
	if !ok {
		panic("invalid minRole specified for RBAC middleware: " + string(minRole))
	}

	return func(c *gin.Context) {
		tcVal, exists := c.Get("tenant_ctx")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "tenant context missing"})
			return
		}

		tc, ok := tcVal.(storage.TenantContext)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "invalid tenant context"})
			return
		}

		userWeight, ok := roleWeights[tc.UserRole]
		if !ok || userWeight < minWeight {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":    "insufficient project permissions",
				"required": string(minRole),
				"current":  tc.UserRole,
			})
			return
		}

		c.Next()
	}
}
