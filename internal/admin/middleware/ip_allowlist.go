package middleware

import (
	"elitegate/internal/ipfilter"
	"net/http"

	"github.com/gin-gonic/gin"
)

func AdminIPAllowlist(checker *ipfilter.IPChecker, trustProxy bool) gin.HandlerFunc {
	if checker == nil {
		panic("AdminIPAllowlist: IPChecker must not be nil")
	}
	return func(c *gin.Context) {
		clientIP := ipfilter.ExtractIP(c.Request.RemoteAddr, c.GetHeader, trustProxy)
		if !checker.IsAllowed(clientIP) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "IP access forbidden"})
			return
		}
		c.Next()
	}
}
