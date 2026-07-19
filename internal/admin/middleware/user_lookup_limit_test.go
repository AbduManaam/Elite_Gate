package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"elitegate/internal/ratelimit"
)

func TestUserLookupRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	limiter := ratelimit.NewMemoryLimiter(2)

	r.GET("/lookup", func(c *gin.Context) {
		// Mock AdminAuth setting the AdminUserIDKey
		c.Set(AdminUserIDKey, "user-123")
		c.Next()
	}, UserLookupRateLimit(limiter, 2), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// 1st request -> Allowed
	req, _ := http.NewRequest(http.MethodGet, "/lookup", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 2nd request -> Allowed
	req, _ = http.NewRequest(http.MethodGet, "/lookup", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 3rd request -> Blocked
	req, _ = http.NewRequest(http.MethodGet, "/lookup", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}
