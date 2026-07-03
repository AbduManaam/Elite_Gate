package middleware

import (
	"fmt"
	"net/http"

	"elitegate/internal/ratelimit"

	"github.com/gin-gonic/gin"
)

func UserLookupRateLimit(limiter ratelimit.Limiter, rpm int) gin.HandlerFunc {
	return func(c *gin.Context) {
		callerID, _ := c.Get(AdminUserIDKey)
		key := fmt.Sprintf("user-lookup:%v", callerID)

		if !limiter.AllowWithLimit(key, rpm) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many lookup requests, slow down",
			})
			return
		}
		c.Next()
	}
}
