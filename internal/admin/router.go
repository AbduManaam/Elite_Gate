package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func NewRouter(logger zerolog.Logger) http.Handler {
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

	v1 := r.Group("/admin/v1")
	{
		routes := v1.Group("/routes")
		{
			routes.GET("", listRoutesHandler)
			routes.POST("", createRouteHandler)
			routes.PUT("/:id", updateRouteHandler)
			routes.DELETE("/:id", deleteRouteHandler)
		}
	}

	logger.Debug().Msg("admin router configured")
	return r
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
