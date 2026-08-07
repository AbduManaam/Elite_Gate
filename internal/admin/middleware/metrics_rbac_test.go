package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"elitegate/internal/admin/middleware"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestMetricsRBAC(t *testing.T) {
	gin.SetMode(gin.TestMode)

	projectIDA := uuid.New()
	projectIDB := uuid.New()
	userID := uuid.New()

	setupEngine := func(minRole middleware.Role) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			// Simulate AdminAuth session extraction
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
				return
			}
			c.Set(middleware.AdminUserIDKey, userID.String())
			c.Next()
		})

		r.GET("/admin/v1/projects/:projectId/metrics/system", func(c *gin.Context) {
			pID := c.Param("projectId")
			// Simulate ProjectScope membership check
			if pID != projectIDA.String() {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "access denied to project"})
				return
			}
			// Simulate TenantContext setting from membership DB query
			roleStr := c.GetHeader("X-Test-Role")
			c.Set("tenant_ctx", storage.TenantContext{
				ProjectID: projectIDA,
				UserID:    userID,
				UserRole:  roleStr,
			})
			c.Next()
		}, middleware.RBAC(minRole), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		r.GET("/admin/v1/projects/:projectId/metrics/system/range", func(c *gin.Context) {
			pID := c.Param("projectId")
			if pID != projectIDA.String() {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "access denied to project"})
				return
			}
			roleStr := c.GetHeader("X-Test-Role")
			c.Set("tenant_ctx", storage.TenantContext{
				ProjectID: projectIDA,
				UserID:    userID,
				UserRole:  roleStr,
			})
			c.Next()
		}, middleware.RBAC(minRole), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		return r
	}

	engine := setupEngine(middleware.RoleViewer)

	t.Run("Unauthenticated request returns 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/v1/projects/"+projectIDA.String()+"/metrics/system", nil)
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Viewer role + project member returns 200 for /system", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/v1/projects/"+projectIDA.String()+"/metrics/system", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("X-Test-Role", "viewer")
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Viewer role + project member returns 200 for /system/range", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/v1/projects/"+projectIDA.String()+"/metrics/system/range", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("X-Test-Role", "viewer")
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Editor role + project member returns 200", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/v1/projects/"+projectIDA.String()+"/metrics/system", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("X-Test-Role", "editor")
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Owner role + project member returns 200", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/v1/projects/"+projectIDA.String()+"/metrics/system", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("X-Test-Role", "owner")
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("User requesting another project returns 403", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/v1/projects/"+projectIDB.String()+"/metrics/system", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("X-Test-Role", "viewer")
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}
