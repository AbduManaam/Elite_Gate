package gateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"elitegate/internal/config"
	"elitegate/internal/gateway/health"
	"elitegate/internal/gateway/middleware"
	"elitegate/internal/gateway/runtime"
	"elitegate/internal/model"
	"elitegate/internal/ratelimit"

	"github.com/rs/zerolog"
)

func TestGatewayRouter_EndToEnd(t *testing.T) {
	logger := zerolog.Nop()

	// Upstream test server capturing incoming requests
	var lastUpstreamPath string
	var lastUpstreamQuery string
	var lastUpstreamMethod string
	var lastUpstreamBody string
	var lastUpstreamAuth string
	var lastUpstreamContentType string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastUpstreamPath = r.URL.Path
		lastUpstreamQuery = r.URL.RawQuery
		lastUpstreamMethod = r.Method
		lastUpstreamAuth = r.Header.Get("Authorization")
		lastUpstreamContentType = r.Header.Get("Content-Type")

		b, _ := io.ReadAll(r.Body)
		lastUpstreamBody = string(b)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"upstream":"ok"}`))
	}))
	defer upstream.Close()

	routes := []model.Route{
		{ID: "r_home", Path: "/home", UpstreamURL: upstream.URL, Protocol: "http", Enabled: true, MatchType: "exact"},
		{ID: "r_products_root", Path: "/products/", UpstreamURL: upstream.URL, Protocol: "http", Enabled: true, MatchType: "prefix"},
		{ID: "r_products_filter", Path: "/products/filter", UpstreamURL: upstream.URL, Protocol: "http", Enabled: true, MatchType: "exact"},
		{ID: "r_products_id", Path: "/products/:id", UpstreamURL: upstream.URL, Protocol: "http", Enabled: true, MatchType: "exact"},
		{ID: "r_api_addresses", Path: "/api/addresses", UpstreamURL: upstream.URL, Protocol: "http", Enabled: true, MatchType: "exact"},
		{ID: "r_orders", Path: "/api/orders", UpstreamURL: upstream.URL, Protocol: "http", Enabled: true, MatchType: "prefix", AllowedOrigins: []string{"*"}},
	}

	snap := &runtime.Snapshot{
		Routes: routes,
	}

	mockControl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"project_id":"00000000-0000-0000-0000-000000000000","routes":[]}`))
	}))
	defer mockControl.Close()

	controlClient := runtime.NewControlPlaneClient(mockControl.URL, "00000000-0000-0000-0000-000000000000", "token", logger)
	loader := runtime.NewLoader(controlClient, nil, logger, 10*time.Second)
	loader.SetSnapshotForTest(snap)

	cfg := &config.Config{
		Server: config.ServerConfig{
			AllowedOrigins: []string{"*"},
		},
		Auth: config.AuthConfig{
			GatewaySyncToken: "secret_reload_token",
		},
		RateLimit: config.RateLimitConfig{
			RequestsPerMinute: 1000,
		},
	}

	authMiddleware := middleware.NewAuthMiddleware(nil, nil, &logger)
	limiter := ratelimit.NewMemoryLimiter(1000)
	hc := health.New(10*time.Second, 2*time.Second, logger)

	router, err := NewRouter(logger, nil, cfg, loader, authMiddleware, limiter, hc)
	if err != nil {
		t.Fatalf("failed to create router: %v", err)
	}

	t.Run("GET /health returns EliteGate health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("gateway")) {
			t.Errorf("expected gateway health JSON, got %s", rec.Body.String())
		}
	})

	t.Run("GET /home reaches upstream with preserved path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/home", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if lastUpstreamPath != "/home" {
			t.Errorf("expected upstream path /home, got %s", lastUpstreamPath)
		}
	})

	t.Run("GET /products/ reaches upstream", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/products/", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if lastUpstreamPath != "/products/" {
			t.Errorf("expected upstream path /products/, got %s", lastUpstreamPath)
		}
	})

	t.Run("GET /products/filter preserves query params", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/products/filter?page=1&limit=8", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if lastUpstreamPath != "/products/filter" {
			t.Errorf("expected upstream path /products/filter, got %s", lastUpstreamPath)
		}
		if lastUpstreamQuery != "page=1&limit=8" {
			t.Errorf("expected query page=1&limit=8, got %s", lastUpstreamQuery)
		}
	})

	t.Run("GET /products/44 matches /products/:id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/products/44", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if lastUpstreamPath != "/products/44" {
			t.Errorf("expected upstream path /products/44, got %s", lastUpstreamPath)
		}
	})

	t.Run("GET /api/addresses still works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/addresses", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if lastUpstreamPath != "/api/addresses" {
			t.Errorf("expected upstream path /api/addresses, got %s", lastUpstreamPath)
		}
	})

	t.Run("POST request body, Authorization, Content-Type preserved", func(t *testing.T) {
		bodyPayload := `{"name":"test_product","price":99}`
		req := httptest.NewRequest(http.MethodPost, "/products/", bytes.NewBufferString(bodyPayload))
		req.Header.Set("Authorization", "Bearer token_abc")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if lastUpstreamMethod != "POST" {
			t.Errorf("expected POST method at upstream, got %s", lastUpstreamMethod)
		}
		if lastUpstreamBody != bodyPayload {
			t.Errorf("expected body %s, got %s", bodyPayload, lastUpstreamBody)
		}
		if lastUpstreamAuth != "Bearer token_abc" {
			t.Errorf("expected auth header Bearer token_abc, got %s", lastUpstreamAuth)
		}
		if lastUpstreamContentType != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", lastUpstreamContentType)
		}
	})

	t.Run("OPTIONS CORS preflight works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/orders", nil)
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("expected 204 No Content for CORS preflight, got %d", rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") == "" {
			t.Errorf("expected Access-Control-Allow-Origin header")
		}
	})

	t.Run("Anonymous /reload fails with 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/reload", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for anonymous /reload, got %d", rec.Code)
		}
	})

	t.Run("Authorized /reload succeeds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/reload", nil)
		req.Header.Set("X-Internal-Token", "secret_reload_token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for authorized /reload, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Unknown route returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/unknown/nonexistent", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 for unknown route, got %d", rec.Code)
		}
	})
}
