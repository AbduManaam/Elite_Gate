package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"elitegate/internal/auth"
	"elitegate/internal/model"
)

func TestControlPlaneClient(t *testing.T) {
	logger := zerolog.Nop()
	projectID := uuid.New()
	token := "derived_jwt_secret_token"

	t.Run("FetchSnapshot Success", func(t *testing.T) {
		expectedSnapshot := TenantSnapshot{
			ProjectID: projectID,
			Routes: []model.Route{
				{ID: "route-1", Path: "/api/v1/users"},
			},
			Upstreams: []model.Upstream{
				{ID: "upstream-1", Name: "users-service"},
			},
			Targets: map[string][]model.UpstreamTarget{
				"upstream-1": {
					{ID: "target-1", TargetURL: "http://localhost:8081", Weight: 1, Enabled: true},
				},
			},
			APIKeys: []TenantAPIKeyDTO{
				{KeyHash: "hash-123", Roles: []string{"viewer"}, Scopes: []string{"read"}},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET request, got %s", r.Method)
			}
			if r.Header.Get("X-Gateway-Token") != token {
				t.Errorf("expected gateway token %s, got %s", token, r.Header.Get("X-Gateway-Token"))
			}
			expectedURL := "/internal/v1/projects/" + projectID.String() + "/sync"
			if r.URL.Path != expectedURL {
				t.Errorf("expected path %s, got %s", expectedURL, r.URL.Path)
			}

			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(expectedSnapshot)
		}))
		defer server.Close()

		client := NewControlPlaneClient(server.URL, projectID.String(), token, logger)
		snapshot, err := client.FetchSnapshot(context.Background())
		if err != nil {
			t.Fatalf("unexpected fetch error: %v", err)
		}

		if snapshot.ProjectID != projectID {
			t.Errorf("expected project ID %s, got %s", projectID, snapshot.ProjectID)
		}
		if len(snapshot.Routes) != 1 || snapshot.Routes[0].ID != "route-1" {
			t.Errorf("unexpected routes fetched")
		}
		if len(snapshot.APIKeys) != 1 || snapshot.APIKeys[0].KeyHash != "hash-123" {
			t.Errorf("unexpected API keys fetched")
		}
	})

	t.Run("FetchSnapshot Unreachable Server", func(t *testing.T) {
		client := NewControlPlaneClient("http://localhost:9999", projectID.String(), token, logger)
		_, err := client.FetchSnapshot(context.Background())
		if err == nil {
			t.Fatal("expected connection error, got nil")
		}
	})
}

func TestWarmAPIKeyCache(t *testing.T) {
	logger := zerolog.Nop()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	loader := NewLoader(nil, rdb, logger, 10*time.Second)

	t.Run("Warm Cache with Valid Keys", func(t *testing.T) {
		keys := []TenantAPIKeyDTO{
			{KeyHash: "keyhash123", Roles: []string{"editor"}, Scopes: []string{"write"}},
		}

		loader.WarmAPIKeyCache(context.Background(), keys)

		// Check miniredis directly
		// Note: since os.Getenv("REDIS_PREFIX") is empty in tests, key is simply "apikey:keyhash123"
		data, err := mr.Get("apikey:keyhash123")
		if err != nil {
			t.Fatalf("expected key in redis, got error: %v", err)
		}

		var rec auth.APIKeyRecord
		if err := json.Unmarshal([]byte(data), &rec); err != nil {
			t.Fatalf("failed to unmarshal redis data: %v", err)
		}

		if len(rec.Roles) != 1 || rec.Roles[0] != "editor" {
			t.Errorf("unexpected roles: %v", rec.Roles)
		}
	})
}
