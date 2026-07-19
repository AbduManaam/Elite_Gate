package middleware

import (
	"errors"
	"net/http"

	"elitegate/helper"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleOwner  Role = "owner"
)

// This middleware ensures that a logged-in user can only access projects they are members of.
// Incoming Request (e.g., GET /admin/v1/projects/project-123/routes)
//    │
//    ▼
// [Check 1] Extract project ID from the URL parameter ("project-123")
//    │
//    ▼
// [Check 2] Get the logged-in user's ID from the session context
//    │
//    ▼
// [Check 3] Query Database: Is this user a member of "project-123"?
//    ├── No  ──▶ Abort with "403 Access Denied to Project"
//    └── Yes ──▶ Get user's role (owner, editor, or viewer)
//          │
//          ▼
// [Check 4] Is the project active (not suspended)?
//    ├── No  ──▶ Abort with "403 project suspended"
//    └── Yes ──▶ Set TenantContext containing (ProjectID, UserID, UserRole)
//          │
//          ▼
// [Action] Attach context to Go's Request Context (this triggers Postgres RLS)
//          │
//          ▼
//        c.Next() (Move to next middleware / handler)

// By injecting the TenantContext into the Go request context, this middleware works directly
// with your database storage repositories (withTenantTx). This ensures that PostgreSQL Row-Level Security (RLS)
// restricts queries to only return data belonging to that specific project_id.

// ProjectScope validates that the calling admin belongs to the project
// in the URL, sets tenant context for RLS, and rejects requests against
// a suspended project — even with an otherwise-valid JWT and membership.
func ProjectScope(membershipRepo *storage.MembershipRepo, projectRepo *storage.ProjectRepo) gin.HandlerFunc {
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
			helper.RespondInternalError(c, zerolog.Nop(), err, "internal error")
			return
		}

		// Suspension check — runs AFTER membership validation so a
		// suspended project still correctly returns 403 even for its
		// own members, including a valid, unexpired JWT issued before
		// the suspension. The token's cryptographic validity is
		// irrelevant here; the DB flag is the source of truth.
		active, err := projectRepo.IsActive(c.Request.Context(), projectID.String())
		if err != nil {
			if errors.Is(err, storage.ErrProjectNotFound) {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
				return
			}
			helper.RespondInternalError(c, zerolog.Nop(), err, "internal error")
			return
		}
		if !active {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "project suspended"})
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

// Once we know the user is allowed to access the project, the RBAC middleware
// ensures they have the correct permissions for the action they are trying to perform.
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
			helper.RespondInternalError(c, zerolog.Nop(), nil, "invalid tenant context")
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
