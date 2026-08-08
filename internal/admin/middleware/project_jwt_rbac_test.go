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

func TestProjectJWTRBAC(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	projectA := uuid.New()
	projectB := uuid.New()
	userID := uuid.New()

	r := gin.New()

	// Simulates AdminAuth.
	r.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatusJSON(
				http.StatusUnauthorized,
				gin.H{
					"error": "unauthenticated",
				},
			)
			return
		}

		c.Set(
			middleware.AdminUserIDKey,
			userID.String(),
		)

		c.Next()
	})

	projectScope := func(
		c *gin.Context,
	) {
		projectID :=
			c.Param("projectId")

		if projectID != projectA.String() {
			c.AbortWithStatusJSON(
				http.StatusForbidden,
				gin.H{
					"error": "access denied to project",
				},
			)
			return
		}

		role :=
			c.GetHeader("X-Test-Role")

		tc := storage.TenantContext{
			ProjectID: projectA,
			UserID:    userID,
			UserRole:  role,
		}

		c.Set(
			"tenant_ctx",
			tc,
		)

		ctx :=
			storage.WithTenantContext(
				c.Request.Context(),
				tc,
			)

		c.Request =
			c.Request.WithContext(ctx)

		c.Next()
	}

	register := func(
		method string,
		path string,
	) {
		r.Handle(
			method,
			path,
			projectScope,
			middleware.RBAC(
				middleware.RoleOwner,
			),
			func(c *gin.Context) {
				c.JSON(
					http.StatusOK,
					gin.H{
						"status": "ok",
					},
				)
			},
		)
	}

	base :=
		"/admin/v1/projects/:projectId/security/jwt"

	register(
		http.MethodGet,
		base,
	)

	register(
		http.MethodPut,
		base,
	)

	register(
		http.MethodDelete,
		base,
	)

	t.Run(
		"unauthenticated returns 401",
		func(t *testing.T) {
			req :=
				httptest.NewRequest(
					http.MethodGet,
					"/admin/v1/projects/"+
						projectA.String()+
						"/security/jwt",
					nil,
				)

			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(
				t,
				http.StatusUnauthorized,
				w.Code,
			)
		},
	)

	t.Run(
		"viewer returns 403",
		func(t *testing.T) {
			req :=
				httptest.NewRequest(
					http.MethodGet,
					"/admin/v1/projects/"+
						projectA.String()+
						"/security/jwt",
					nil,
				)

			req.Header.Set(
				"Authorization",
				"Bearer test",
			)

			req.Header.Set(
				"X-Test-Role",
				"viewer",
			)

			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(
				t,
				http.StatusForbidden,
				w.Code,
			)
		},
	)

	t.Run(
		"editor returns 403",
		func(t *testing.T) {
			req :=
				httptest.NewRequest(
					http.MethodPut,
					"/admin/v1/projects/"+
						projectA.String()+
						"/security/jwt",
					nil,
				)

			req.Header.Set(
				"Authorization",
				"Bearer test",
			)

			req.Header.Set(
				"X-Test-Role",
				"editor",
			)

			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(
				t,
				http.StatusForbidden,
				w.Code,
			)
		},
	)

	t.Run(
		"owner can access all JWT operations",
		func(t *testing.T) {
			methods := []string{
				http.MethodGet,
				http.MethodPut,
				http.MethodDelete,
			}

			for _, method := range methods {

				req :=
					httptest.NewRequest(
						method,
						"/admin/v1/projects/"+
							projectA.String()+
							"/security/jwt",
						nil,
					)

				req.Header.Set(
					"Authorization",
					"Bearer test",
				)

				req.Header.Set(
					"X-Test-Role",
					"owner",
				)

				w :=
					httptest.NewRecorder()

				r.ServeHTTP(
					w,
					req,
				)

				assert.Equal(
					t,
					http.StatusOK,
					w.Code,
					method,
				)
			}
		},
	)

	t.Run(
		"owner cannot access another project",
		func(t *testing.T) {
			req :=
				httptest.NewRequest(
					http.MethodGet,
					"/admin/v1/projects/"+
						projectB.String()+
						"/security/jwt",
					nil,
				)

			req.Header.Set(
				"Authorization",
				"Bearer test",
			)

			req.Header.Set(
				"X-Test-Role",
				"owner",
			)

			w :=
				httptest.NewRecorder()

			r.ServeHTTP(
				w,
				req,
			)

			assert.Equal(
				t,
				http.StatusForbidden,
				w.Code,
			)
		},
	)
}
