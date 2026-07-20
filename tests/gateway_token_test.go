package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"elitegate/helper"
	"elitegate/internal/admin/middleware"
)

func TestRequireGatewayToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	masterSecret := "supersecretjwtkey_32byteslongkey!"
	projectA := "00000000-0000-0000-0000-00000000000a"
	projectB := "00000000-0000-0000-0000-00000000000b"

	tokenA := helper.DeriveTenantJWTSecret(masterSecret, projectA)
	tokenB := helper.DeriveTenantJWTSecret(masterSecret, projectB)

	setupRouter := func() *gin.Engine {
		r := gin.New()
		r.GET("/internal/v1/projects/:project_id/sync", middleware.RequireGatewayToken(masterSecret), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
		return r
	}

	t.Run("Valid Token for Project A", func(t *testing.T) {
		r := setupRouter()
		req, _ := http.NewRequest(http.MethodGet, "/internal/v1/projects/"+projectA+"/sync", nil)
		req.Header.Set("X-Gateway-Token", tokenA)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("Invalid Token for Project A using B's Token", func(t *testing.T) {
		r := setupRouter()
		req, _ := http.NewRequest(http.MethodGet, "/internal/v1/projects/"+projectA+"/sync", nil)
		req.Header.Set("X-Gateway-Token", tokenB)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("Missing Header", func(t *testing.T) {
		r := setupRouter()
		req, _ := http.NewRequest(http.MethodGet, "/internal/v1/projects/"+projectA+"/sync", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("Missing Project ID in route", func(t *testing.T) {
		r := gin.New()
		r.GET("/internal/v1/sync", middleware.RequireGatewayToken(masterSecret), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})

		req, _ := http.NewRequest(http.MethodGet, "/internal/v1/sync", nil)
		req.Header.Set("X-Gateway-Token", tokenA)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}
