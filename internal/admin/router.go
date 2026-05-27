package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"elitegate/internal/admin/handler"
	"elitegate/internal/admin/middleware"
	"elitegate/internal/auth"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func NewRouter(logger zerolog.Logger, db *sql.DB, jwtSecret string) (http.Handler, error) {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "admin-api",
		})
	})

	r.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
		})
	})

	adminTokens, err := auth.NewAdminTokenManager(jwtSecret, "elitegate-admin")
	if err != nil {
		return nil, fmt.Errorf("create admin token manager: %w", err)
	}
	authRepo := storage.NewAdminAuthRepo(db)
	loginLimiter := middleware.NewLoginRateLimiter(5, time.Minute)
	authHandler := handler.NewAuthHandler(authRepo, adminTokens, loginLimiter, logger)

	r.POST("/admin/login", authHandler.Login)
	r.POST("/admin/refresh", authHandler.Refresh)
	r.POST("/admin/logout", authHandler.Logout)

	// ── Public bootstrap registration ─────────────────────────────────
	// Only works when 0 admin users exist in the database.
	// After the first admin is created, returns 403 Forbidden.
	r.POST("/admin/register", authHandler.Register)

	v1 := r.Group("/admin/v1")
	v1.Use(middleware.AdminAuth(adminTokens))
	{
		routes := v1.Group("/routes")
		{
			routes.GET("", listRoutesHandler)
			routes.POST("", createRouteHandler)
			routes.PUT("/:id", updateRouteHandler)
			routes.DELETE("/:id", deleteRouteHandler)
		}

		// ── Protected admin account management ────────────────────────
		// Requires a valid admin JWT. Existing admin creates new admins.
		admins := v1.Group("/admins")
		{
			admins.POST("", authHandler.Register)
		}
	}

	logger.Debug().Msg("admin router configured")
	return r, nil
}

func listRoutesHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"routes": []interface{}{}})
}

func createRouteHandler(c *gin.Context) {
	c.JSON(http.StatusCreated, gin.H{"message": "created"})
}

func updateRouteHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func deleteRouteHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
