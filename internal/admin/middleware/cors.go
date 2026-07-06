package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// DynamicCORS checks a static origin list (for routes with no project context)
// and a per-project cached origin list (for project-scoped routes).
func DynamicCORS(staticOrigins []string, cache *OriginCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}

		allowed := contains(staticOrigins, origin)

		if !allowed && cache != nil {
			if projectID := c.Param("projectId"); projectID != "" {
				if origins, err := cache.Get(c.Request.Context(), projectID); err == nil {
					allowed = contains(origins, origin)
				}
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Vary", "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			if allowed {
				c.AbortWithStatus(http.StatusNoContent)
			} else {
				c.AbortWithStatus(http.StatusForbidden)
			}
			return
		}
		c.Next()
	}
}
