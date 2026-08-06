package router

import (
	"testing"

	"elitegate/internal/model"
)

func TestMatchHTTP_ParameterizedAndBoundaryPrefix(t *testing.T) {
	routes := []model.Route{
		{ID: "r1", Path: "/home", Protocol: "http", Enabled: true, MatchType: "exact"},
		{ID: "r2", Path: "/products/", Protocol: "http", Enabled: true, MatchType: "prefix"},
		{ID: "r3", Path: "/products/filter", Protocol: "http", Enabled: true, MatchType: "exact"},
		{ID: "r4", Path: "/products/:id", Protocol: "http", Enabled: true, MatchType: "exact"},
		{ID: "r5", Path: "/categories/", Protocol: "http", Enabled: true, MatchType: "prefix"},
		{ID: "r6", Path: "/api/addresses", Protocol: "http", Enabled: true, MatchType: "exact"},
		{ID: "r7", Path: "/api/orders", Protocol: "http", Enabled: true, MatchType: "prefix"},
	}

	tests := []struct {
		name     string
		path     string
		method   string
		expected string // route ID or ""
	}{
		{"exact /home", "/home", "GET", "r1"},
		{"prefix /products/", "/products/", "GET", "r2"},
		{"exact /products/filter", "/products/filter", "GET", "r3"},
		{"param /products/44", "/products/44", "GET", "r4"},
		{"param child /products/44/details should NOT match /products/:id", "/products/44/details", "GET", "r2"}, // falls back to prefix /products/
		{"prefix /categories/", "/categories/", "GET", "r5"},
		{"exact /api/addresses", "/api/addresses", "GET", "r6"},
		{"prefix /api/orders match /api/orders/10", "/api/orders/10", "GET", "r7"},
		{"boundary prefix /api/orders-old should NOT match /api/orders", "/api/orders-old", "GET", ""},
		{"unrelated path no match", "/unrelated/path", "GET", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := MatchHTTP(tt.path, tt.method, routes)
			if tt.expected == "" {
				if matched != nil {
					t.Errorf("expected nil match for %s, got %s", tt.path, matched.ID)
				}
			} else {
				if matched == nil || matched.ID != tt.expected {
					got := "nil"
					if matched != nil {
						got = matched.ID
					}
					t.Errorf("expected route %s for %s, got %s", tt.expected, tt.path, got)
				}
			}
		})
	}
}

func TestMatchHTTP_HTTPMethodFiltering(t *testing.T) {
	routes := []model.Route{
		{ID: "r_get", Path: "/items", Methods: []string{"GET"}, Protocol: "http", Enabled: true, MatchType: "exact"},
		{ID: "r_post", Path: "/items", Methods: []string{"POST"}, Protocol: "http", Enabled: true, MatchType: "exact"},
	}

	if m := MatchHTTP("/items", "GET", routes); m == nil || m.ID != "r_get" {
		t.Errorf("expected r_get for GET /items, got %v", m)
	}

	if m := MatchHTTP("/items", "POST", routes); m == nil || m.ID != "r_post" {
		t.Errorf("expected r_post for POST /items, got %v", m)
	}

	if m := MatchHTTP("/items", "DELETE", routes); m != nil {
		t.Errorf("expected nil match for DELETE /items, got %s", m.ID)
	}

	// OPTIONS preflight should be allowed to match route for CORS handling
	if m := MatchHTTP("/items", "OPTIONS", routes); m == nil {
		t.Errorf("expected match for OPTIONS /items preflight")
	}
}
