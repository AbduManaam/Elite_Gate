package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"elitegate/internal/ratelimit"
)

func TestIPRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Standard Rate Limiting by IP", func(t *testing.T) {
		r := gin.New()
		limiter := ratelimit.NewMemoryLimiter(2)

		r.POST("/test", IPRateLimit(limiter, 2, "test", false), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		// 1st request from IP1
		req, _ := http.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if remaining := w.Header().Get("X-RateLimit-Remaining"); remaining != "1" {
			t.Errorf("expected remaining 1, got %s", remaining)
		}

		// 2nd request from IP1
		req, _ = http.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if remaining := w.Header().Get("X-RateLimit-Remaining"); remaining != "0" {
			t.Errorf("expected remaining 0, got %s", remaining)
		}

		// 3rd request from IP1 -> Blocked
		req, _ = http.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429, got %d", w.Code)
		}
		if retryAfter := w.Header().Get("Retry-After"); retryAfter != "60" {
			t.Errorf("expected Retry-After 60, got %s", retryAfter)
		}

		// 4th request from IP2 -> Allowed (shows key isolation)
		req, _ = http.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "192.168.1.2:12345"
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for different IP, got %d", w.Code)
		}
	})

	t.Run("Trust Proxy Headers = True", func(t *testing.T) {
		r := gin.New()
		limiter := ratelimit.NewMemoryLimiter(1)

		r.POST("/test", IPRateLimit(limiter, 1, "test", true), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		// Request with X-Forwarded-For header
		req, _ := http.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345" // proxy IP
		req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18, 150.172.238.178")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		// Second request from same proxy but different client IP in XFF -> Allowed
		req, _ = http.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", "198.51.100.2, 70.41.3.18")
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		// Third request repeating the first client IP -> Blocked
		req, _ = http.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429, got %d", w.Code)
		}
	})

	t.Run("Trust Proxy Headers = False", func(t *testing.T) {
		r := gin.New()
		limiter := ratelimit.NewMemoryLimiter(1)

		r.POST("/test", IPRateLimit(limiter, 1, "test", false), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		// Request with X-Forwarded-For header
		req, _ := http.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.195")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		// Second request from same proxy IP with different XFF -> Blocked (header ignored)
		req, _ = http.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", "198.51.100.2")
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429 because proxy IP matches and XFF was ignored, got %d", w.Code)
		}
	})
}

func TestIPRateLimit_ComposingWithLoginLockout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	loginIPLimiter := ratelimit.NewMemoryLimiter(2)
	loginFailLimiter := NewLoginRateLimiter(2, time.Minute)

	r.POST("/login", IPRateLimit(loginIPLimiter, 2, "login", false), func(c *gin.Context) {
		ip := ClientIP(c)
		if loginFailLimiter.TooManyFailures(ip) {
			c.JSON(http.StatusLocked, gin.H{"error": "locked out"})
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		loginFailLimiter.RecordFailure(ip)
	})

	// 1st request -> returns 401, records 1st failure
	req, _ := http.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// 2nd request -> returns 401, records 2nd failure
	req, _ = http.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// 3rd request -> hits IPRateLimit (cap 2) -> returns 429, bypasses logic
	req, _ = http.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 from IPRateLimit, got %d", w.Code)
	}

	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "too many requests, please slow down" {
		t.Errorf("expected rate limit error message, got %v", body["error"])
	}
}
