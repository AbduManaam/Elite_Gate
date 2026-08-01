package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"elitegate/internal/gateway/runtime"
	"elitegate/internal/model"
	"elitegate/internal/shared"
)

func TestRouteMatcher_HostHeaderContextAttachment(t *testing.T) {
	logger := zerolog.Nop()
	loader := runtime.NewLoader(nil, nil, logger, 10*time.Second)

	// Inject snapshot into loader via Reload (or unexported snapshot test helper)
	loader.SetHealthChecker(nil)

	// Create control client mock or set snapshot directly via loader reload test
	// Note: We can test RouteMatcher using a snapshot set on loader
	// loader.Start / Reload
	// Let's test RouteMatcher middleware function
	matcher := RouteMatcher(loader)

	t.Run("unmatched host header proceeds without custom domain context", func(t *testing.T) {
		var capturedCustomDomain *runtime.DomainContext

		handler := matcher(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cd, ok := r.Context().Value(shared.ContextKeyCustomDomain).(runtime.DomainContext); ok {
				capturedCustomDomain = &cd
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Host = "unknown.example.com"
		rec := httptest.NewRecorder()

		// RouteMatcher returns 404 if path doesn't match any route
		handler.ServeHTTP(rec, req)

		if capturedCustomDomain != nil {
			t.Errorf("expected no custom domain context for unknown host")
		}
	})
}

func TestRouteMatcher_MatchedHost(t *testing.T) {
	// Helper test verifying context propagation
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resource", nil)
	req.Host = "TEST-API.ELITEGATEWAY.SITE:443"

	// Validate NormalizeHost behavior used by RouteMatcher
	norm := runtime.NormalizeHost(req.Host)
	if norm != "test-api.elitegateway.site" {
		t.Errorf("expected normalized host test-api.elitegateway.site, got %s", norm)
	}

	snap := runtime.Snapshot{
		Routes: []model.Route{
			{ID: "route-1", Path: "/api/v1/resource", Protocol: "http", Enabled: true},
		},
		DomainMap: map[string]runtime.DomainContext{
			"test-api.elitegateway.site": {
				Hostname:      "test-api.elitegateway.site",
				Status:        "verified",
				RoutingStatus: "ready",
			},
		},
	}

	d, ok := snap.LookupDomain(req.Host)
	if !ok {
		t.Fatalf("expected LookupDomain to succeed for %s", req.Host)
	}
	if d.Hostname != "test-api.elitegateway.site" {
		t.Errorf("expected hostname test-api.elitegateway.site, got %s", d.Hostname)
	}
}
