package storage

import (
	"testing"
	"time"

	"elitegate/internal/model"
)

func TestSummaryCache(t *testing.T) {
	cache := NewSummaryCache(50 * time.Millisecond)

	projectID := "proj-1"
	summary := &model.ProjectSummary{
		ID:   projectID,
		Name: "Test Project",
	}

	// 1. Get from empty cache
	_, ok := cache.Get(projectID)
	if ok {
		t.Fatal("expected cache miss")
	}

	// 2. Set and Get
	cache.Set(projectID, summary)
	cached, ok := cache.Get(projectID)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if cached.ID != projectID {
		t.Errorf("expected cached project ID %s, got %s", projectID, cached.ID)
	}

	// 3. TTL expiration
	time.Sleep(60 * time.Millisecond)
	_, ok = cache.Get(projectID)
	if ok {
		t.Fatal("expected cache miss after TTL expiration")
	}
}
