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

func TestLoadConfig_GatewayImageName(t *testing.T) {
	origWd, err := os.Getwd()
	if err == nil {
		_ = os.Chdir("../..")
		defer os.Chdir(origWd)
	}

	os.Setenv("POSTGRES_DSN", "postgres://localhost/test")
	os.Setenv("JWT_SECRET", "supersecretjwtkey_32byteslongkey!")
	defer func() {
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("GATEWAY_IMAGE_NAME")
	}()

	// 1. Unset GATEWAY_IMAGE_NAME should default to empty string ""
	os.Unsetenv("GATEWAY_IMAGE_NAME")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Server.GatewayImageName != "" {
		t.Errorf("expected GatewayImageName to be empty when unset, got %q", cfg.Server.GatewayImageName)
	}

	// 2. Set GATEWAY_IMAGE_NAME should populate cfg.Server.GatewayImageName
	customImage := "123456789012.dkr.ecr.us-east-1.amazonaws.com/elitegate-gateway:v1.2.3"
	os.Setenv("GATEWAY_IMAGE_NAME", customImage)
	cfgOverride, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config with GATEWAY_IMAGE_NAME: %v", err)
	}
	if cfgOverride.Server.GatewayImageName != customImage {
		t.Errorf("expected GatewayImageName to be %q, got %q", customImage, cfgOverride.Server.GatewayImageName)
	}
}

func TestLoadConfig_GatewayHostPublic(t *testing.T) {
	origWd, err := os.Getwd()
	if err == nil {
		_ = os.Chdir("../..")
		defer os.Chdir(origWd)
	}

	os.Setenv("POSTGRES_DSN", "postgres://localhost/test")
	os.Setenv("JWT_SECRET", "supersecretjwtkey_32byteslongkey!")
	defer func() {
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("GATEWAY_PUBLIC_HOST")
	}()

	// 1. Unset GATEWAY_PUBLIC_HOST should default to empty string ""
	os.Setenv("GATEWAY_PUBLIC_HOST", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Server.GatewayHostPublic != "" {
		t.Errorf("expected GatewayHostPublic to be empty when unset, got %q", cfg.Server.GatewayHostPublic)
	}

	// 2. Set GATEWAY_PUBLIC_HOST should populate cfg.Server.GatewayHostPublic
	customHost := "gateway.mycompany.com"
	os.Setenv("GATEWAY_PUBLIC_HOST", customHost)
	cfgOverride, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config with GATEWAY_PUBLIC_HOST: %v", err)
	}
	if cfgOverride.Server.GatewayHostPublic != customHost {
		t.Errorf("expected GatewayHostPublic to be %q, got %q", customHost, cfgOverride.Server.GatewayHostPublic)
	}
}

func TestLoadConfig_AWSConfig(t *testing.T) {
	origWd, err := os.Getwd()
	if err == nil {
		_ = os.Chdir("../..")
		defer os.Chdir(origWd)
	}

	os.Setenv("POSTGRES_DSN", "postgres://localhost/test")
	os.Setenv("JWT_SECRET", "supersecretjwtkey_32byteslongkey!")
	defer func() {
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("CUSTOM_DOMAIN_AWS_AUTOMATION_ENABLED")
		os.Unsetenv("AWS_REGION")
		os.Unsetenv("ALB_HTTPS_LISTENER_ARN")
	}()

	// 1. Automation disabled by default
	os.Setenv("CUSTOM_DOMAIN_AWS_AUTOMATION_ENABLED", "false")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config when AWS automation disabled: %v", err)
	}
	if cfg.AWS.AutomationEnabled {
		t.Errorf("expected AutomationEnabled to be false")
	}

	// 2. Automation enabled without listener ARN should fail validation
	os.Setenv("CUSTOM_DOMAIN_AWS_AUTOMATION_ENABLED", "true")
	os.Setenv("ALB_HTTPS_LISTENER_ARN", "")
	_, err = LoadConfig()
	if err == nil {
		t.Fatal("expected LoadConfig error when ALB_HTTPS_LISTENER_ARN is missing")
	}

	// 3. Automation enabled with valid config
	os.Setenv("ALB_HTTPS_LISTENER_ARN", "arn:aws:elasticloadbalancing:ap-south-1:123:listener/456")
	os.Setenv("AWS_REGION", "ap-south-1")
	cfgValid, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config with valid AWS automation env: %v", err)
	}
	if !cfgValid.AWS.AutomationEnabled {
		t.Errorf("expected AutomationEnabled to be true")
	}
	if cfgValid.AWS.Region != "ap-south-1" {
		t.Errorf("expected Region to be ap-south-1, got %q", cfgValid.AWS.Region)
	}
	if cfgValid.AWS.ALBHTTPSListenerARN != "arn:aws:elasticloadbalancing:ap-south-1:123:listener/456" {
		t.Errorf("expected ALBHTTPSListenerARN to match, got %q", cfgValid.AWS.ALBHTTPSListenerARN)
	}
}
