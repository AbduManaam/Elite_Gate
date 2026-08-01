package runtime

import (
	"testing"

	"elitegate/internal/model"
)

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "exact lowercase hostname",
			input:    "test-api.elitegateway.site",
			expected: "test-api.elitegateway.site",
		},
		{
			name:     "uppercase hostname",
			input:    "TEST-API.ELITEGATEWAY.SITE",
			expected: "test-api.elitegateway.site",
		},
		{
			name:     "hostname with standard port 443",
			input:    "test-api.elitegateway.site:443",
			expected: "test-api.elitegateway.site",
		},
		{
			name:     "hostname with custom port 8080",
			input:    "test-api.elitegateway.site:8080",
			expected: "test-api.elitegateway.site",
		},
		{
			name:     "hostname with trailing dot",
			input:    "test-api.elitegateway.site.",
			expected: "test-api.elitegateway.site",
		},
		{
			name:     "uppercase hostname with port and trailing dot",
			input:    "TEST-API.ELITEGATEWAY.SITE:8080.",
			expected: "test-api.elitegateway.site",
		},
		{
			name:     "whitespace with uppercase, port and trailing dot",
			input:    "  TEST-API.ELITEGATEWAY.SITE:8080.  ",
			expected: "test-api.elitegateway.site",
		},
		{
			name:     "empty host",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeHost(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeHost(%q) = %q, expected %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSnapshot_LookupDomain(t *testing.T) {
	snap := Snapshot{
		DomainMap: map[string]DomainContext{
			"test-api.elitegateway.site": {
				Hostname:      "test-api.elitegateway.site",
				Status:        "verified",
				RoutingStatus: "ready",
			},
			"app.mycompany.org": {
				Hostname:      "app.mycompany.org",
				Status:        "verified",
				RoutingStatus: "ready",
			},
		},
	}

	t.Run("exact match", func(t *testing.T) {
		d, ok := snap.LookupDomain("test-api.elitegateway.site")
		if !ok {
			t.Fatalf("expected match for test-api.elitegateway.site")
		}
		if d.Hostname != "test-api.elitegateway.site" {
			t.Errorf("unexpected hostname: %s", d.Hostname)
		}
	})

	t.Run("uppercase with port", func(t *testing.T) {
		d, ok := snap.LookupDomain("TEST-API.ELITEGATEWAY.SITE:443")
		if !ok {
			t.Fatalf("expected match for TEST-API.ELITEGATEWAY.SITE:443")
		}
		if d.Hostname != "test-api.elitegateway.site" {
			t.Errorf("unexpected hostname: %s", d.Hostname)
		}
	})

	t.Run("trailing dot with port", func(t *testing.T) {
		d, ok := snap.LookupDomain("TEST-API.ELITEGATEWAY.SITE:8080.")
		if !ok {
			t.Fatalf("expected match for TEST-API.ELITEGATEWAY.SITE:8080.")
		}
		if d.Hostname != "test-api.elitegateway.site" {
			t.Errorf("unexpected hostname: %s", d.Hostname)
		}
	})

	t.Run("second custom domain", func(t *testing.T) {
		d, ok := snap.LookupDomain("app.mycompany.org:80")
		if !ok {
			t.Fatalf("expected match for app.mycompany.org:80")
		}
		if d.Hostname != "app.mycompany.org" {
			t.Errorf("unexpected hostname: %s", d.Hostname)
		}
	})

	t.Run("unknown domain", func(t *testing.T) {
		_, ok := snap.LookupDomain("unknown.example.com")
		if ok {
			t.Errorf("expected no match for unknown domain")
		}
	})
}

func TestLoader_BuildDomainMap_ActiveOnly(t *testing.T) {
	loader := &Loader{}
	snap := &TenantSnapshot{
		CustomDomains: []model.CustomDomainSync{
			{
				Hostname:      "active-ready.com",
				Status:        "active",
				RoutingStatus: "ready",
			},
			{
				Hostname:      "verified-ready.com",
				Status:        "verified",
				RoutingStatus: "ready",
			},
			{
				Hostname:      "active-pending.com",
				Status:        "active",
				RoutingStatus: "pending",
			},
		},
	}

	domainMap := loader.buildDomainMap(snap)

	if len(domainMap) != 1 {
		t.Fatalf("expected exactly 1 active domain in map, got %d", len(domainMap))
	}

	if _, ok := domainMap["active-ready.com"]; !ok {
		t.Errorf("expected active-ready.com to be present in domainMap")
	}

	if _, ok := domainMap["verified-ready.com"]; ok {
		t.Errorf("expected verified-ready.com to be excluded from domainMap")
	}

	if _, ok := domainMap["active-pending.com"]; ok {
		t.Errorf("expected active-pending.com to be excluded from domainMap")
	}
}
