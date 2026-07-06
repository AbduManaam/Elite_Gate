package config

import (
	"os"
	"testing"
)

func TestLoadConfig_AllowedOrigins(t *testing.T) {
	// Change working directory to project root so relative config paths resolve correctly
	origWd, err := os.Getwd()
	if err == nil {
		_ = os.Chdir("../..")
		defer os.Chdir(origWd)
	}

	// Set required env variables so LoadConfig validation passes
	os.Setenv("POSTGRES_DSN", "postgres://localhost/test")
	os.Setenv("JWT_SECRET", "supersecretjwtkey_32byteslongkey!")
	defer func() {
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("ALLOWED_ORIGINS")
	}()

	// 1. Test allowed_origins loaded from config.yaml
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.Server.AllowedOrigins) == 0 {
		t.Fatal("expected allowed_origins to be populated from config.yaml")
	}

	// 2. Test ALLOWED_ORIGINS environment variable override (comma-separated list)
	os.Setenv("ALLOWED_ORIGINS", "http://env-origin1.local, https://env-origin2.local")
	cfgOverride, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config with ALLOWED_ORIGINS override: %v", err)
	}

	expected := []string{"http://env-origin1.local", "https://env-origin2.local"}
	if len(cfgOverride.Server.AllowedOrigins) != len(expected) {
		t.Fatalf("expected %d origins, got %d", len(expected), len(cfgOverride.Server.AllowedOrigins))
	}
	for i, v := range expected {
		if cfgOverride.Server.AllowedOrigins[i] != v {
			t.Errorf("expected allowed origin at index %d to be %q, got %q", i, v, cfgOverride.Server.AllowedOrigins[i])
		}
	}
}
