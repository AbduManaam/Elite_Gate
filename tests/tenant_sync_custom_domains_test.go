package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"elitegate/internal/admin/handler"
	"elitegate/internal/model"
)

func TestTenantSyncHandler_GetTenantSnapshot_CustomDomains(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := initTenantSyncMockDB(t)
	defer db.Close()

	h := handler.NewTenantSyncHandler(db, zerolog.Nop())

	r := gin.New()
	r.GET("/projects/:project_id/sync", h.GetTenantSnapshot)

	projectID := uuid.New()
	req, _ := http.NewRequest(http.MethodGet, "/projects/"+projectID.String()+"/sync", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var response struct {
		ProjectID     uuid.UUID                `json:"project_id"`
		Routes        []model.Route            `json:"routes"`
		Upstreams     []model.Upstream         `json:"upstreams"`
		CustomDomains []model.CustomDomainSync `json:"custom_domains"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal sync response: %v", err)
	}

	if response.ProjectID != projectID {
		t.Errorf("expected project ID %s, got %s", projectID, response.ProjectID)
	}

	if response.CustomDomains == nil {
		t.Errorf("expected custom_domains to be non-nil empty array []")
	}

	rawJSON := w.Body.String()
	if !containsSubstring(rawJSON, `"custom_domains":[]`) && !containsSubstring(rawJSON, `"custom_domains": []`) {
		t.Errorf("expected JSON to contain 'custom_domains': [], got: %s", rawJSON)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && searchSubstring(s, substr))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
