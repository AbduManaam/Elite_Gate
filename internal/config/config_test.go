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

func TestLoadConfigForService_UnknownService_ReturnsError(t *testing.T) {
	_, err := LoadConfigForService("unknown_service")
	if err == nil {
		t.Fatal("expected error when passing unknown service type, got nil")
	}
}

func TestLoadConfigForService_ProductionMailValidation(t *testing.T) {
	origWd, err := os.Getwd()
	if err == nil {
		_ = os.Chdir("../..")
		defer os.Chdir(origWd)
	}

	os.Setenv("POSTGRES_DSN", "postgres://localhost/test")
	os.Setenv("JWT_SECRET", "supersecretjwtkey_32byteslongkey!")
	os.Setenv("APP_ENV", "production")
	os.Setenv("SMTP_ENABLED", "false")

	defer func() {
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("APP_ENV")
		os.Unsetenv("SMTP_ENABLED")
		os.Unsetenv("SMTP_HOST")
		os.Unsetenv("SMTP_PORT")
		os.Unsetenv("SMTP_FROM_EMAIL")
	}()

	// 1. ServiceAdmin in production with SMTP disabled must return error
	_, err = LoadConfigForService(ServiceAdmin)
	if err == nil {
		t.Fatal("expected error loading Admin config in production when SMTP is disabled")
	}

	// 2. ServiceGateway in production with SMTP disabled must succeed
	cfgGateway, err := LoadConfigForService(ServiceGateway)
	if err != nil {
		t.Fatalf("expected ServiceGateway to succeed in production with SMTP disabled, got error: %v", err)
	}
	if cfgGateway.Mail.Enabled {
		t.Errorf("expected Mail.Enabled to be false for ServiceGateway")
	}

	// 3. ServiceWorker in production with SMTP disabled must succeed
	cfgWorker, err := LoadConfigForService(ServiceWorker)
	if err != nil {
		t.Fatalf("expected ServiceWorker to succeed in production with SMTP disabled, got error: %v", err)
	}
	if cfgWorker.Mail.Enabled {
		t.Errorf("expected Mail.Enabled to be false for ServiceWorker")
	}

	// 4. ServiceGateway in production with SMTP enabled performs full mail field validation
	os.Setenv("SMTP_ENABLED", "true")
	os.Setenv("SMTP_HOST", "") // empty host should fail validation
	_, err = LoadConfigForService(ServiceGateway)
	if err == nil {
		t.Fatal("expected error when SMTP is enabled on ServiceGateway but host is empty")
	}
}

func TestLoadConfigForService_DevelopmentMailValidation(t *testing.T) {
	origWd, err := os.Getwd()
	if err == nil {
		_ = os.Chdir("../..")
		defer os.Chdir(origWd)
	}

	os.Setenv("POSTGRES_DSN", "postgres://localhost/test")
	os.Setenv("JWT_SECRET", "supersecretjwtkey_32byteslongkey!")
	os.Setenv("APP_ENV", "development")
	os.Setenv("SMTP_ENABLED", "false")

	defer func() {
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("APP_ENV")
		os.Unsetenv("SMTP_ENABLED")
	}()

	// Development environment with SMTP disabled loads successfully for all services
	for _, s := range []ServiceType{ServiceAdmin, ServiceGateway, ServiceWorker} {
		_, err := LoadConfigForService(s)
		if err != nil {
			t.Errorf("expected %s in development to succeed with SMTP disabled, got: %v", s, err)
		}
	}
}

func TestEmailVerificationURL_ConfigurationAndValidation(t *testing.T) {
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
		os.Unsetenv("EMAIL_VERIFICATION_URL")
		os.Unsetenv("PASSWORD_RESET_URL")
		os.Unsetenv("APP_ENV")
		os.Unsetenv("SMTP_ENABLED")
		os.Unsetenv("SMTP_HOST")
		os.Unsetenv("SMTP_PORT")
		os.Unsetenv("SMTP_FROM_EMAIL")
	}()

	// 1. Default development URL is http://localhost:5173/verify-email
	os.Unsetenv("EMAIL_VERIFICATION_URL")
	os.Setenv("APP_ENV", "development")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Mail.EmailVerificationURL != "http://localhost:5173/verify-email" {
		t.Errorf("expected default email verification URL to be 'http://localhost:5173/verify-email', got %q", cfg.Mail.EmailVerificationURL)
	}

	// 2. EMAIL_VERIFICATION_URL overrides the default
	overrideURL := "http://custom-dev:3000/verify"
	os.Setenv("EMAIL_VERIFICATION_URL", overrideURL)
	cfgOverride, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config with EMAIL_VERIFICATION_URL override: %v", err)
	}
	if cfgOverride.Mail.EmailVerificationURL != overrideURL {
		t.Errorf("expected EmailVerificationURL to be %q, got %q", overrideURL, cfgOverride.Mail.EmailVerificationURL)
	}

	// 3. Valid HTTPS verification URL works in production
	os.Setenv("APP_ENV", "production")
	os.Setenv("SMTP_ENABLED", "true")
	os.Setenv("SMTP_HOST", "smtp.example.com")
	os.Setenv("SMTP_PORT", "587")
	os.Setenv("SMTP_FROM_EMAIL", "noreply@example.com")
	os.Setenv("PASSWORD_RESET_URL", "https://elitegateway.site/reset-password")
	os.Setenv("EMAIL_VERIFICATION_URL", "https://elitegateway.site/verify-email")
	cfgProd, err := LoadConfigForService(ServiceAdmin)
	if err != nil {
		t.Fatalf("expected valid HTTPS email verification URL in production to succeed, got error: %v", err)
	}
	if cfgProd.Mail.EmailVerificationURL != "https://elitegateway.site/verify-email" {
		t.Errorf("expected production EmailVerificationURL to match set value, got %q", cfgProd.Mail.EmailVerificationURL)
	}

	// 4. HTTP verification URL is rejected in production
	os.Setenv("EMAIL_VERIFICATION_URL", "http://elitegateway.site/verify-email")
	_, err = LoadConfigForService(ServiceAdmin)
	if err == nil {
		t.Fatal("expected error when email verification URL uses HTTP in production")
	}

	// 5. Malformed verification URL is rejected
	os.Setenv("APP_ENV", "development")
	os.Setenv("EMAIL_VERIFICATION_URL", "not-a-valid-url")
	_, err = LoadConfig()
	if err == nil {
		t.Fatal("expected error when email verification URL is malformed")
	}

	// 6. Existing PASSWORD_RESET_URL behavior remains unchanged
	os.Setenv("EMAIL_VERIFICATION_URL", "http://localhost:5173/verify-email")
	os.Setenv("PASSWORD_RESET_URL", "not-a-valid-url")
	_, err = LoadConfig()
	if err == nil {
		t.Fatal("expected error when password reset URL is malformed")
	}
}

func TestResendVerificationRPM_ConfigurationAndValidation(t *testing.T) {
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
		os.Unsetenv("RATE_LIMIT_AUTH_RESEND_VERIFICATION_RPM")
	}()

	// 1. Default resend_verification_rpm is 3
	os.Unsetenv("RATE_LIMIT_AUTH_RESEND_VERIFICATION_RPM")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.RateLimit.Auth.ResendVerificationRPM != 3 {
		t.Errorf("expected default ResendVerificationRPM to be 3, got %d", cfg.RateLimit.Auth.ResendVerificationRPM)
	}

	// 2. Env var override works
	os.Setenv("RATE_LIMIT_AUTH_RESEND_VERIFICATION_RPM", "10")
	cfgOverride, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config with RATE_LIMIT_AUTH_RESEND_VERIFICATION_RPM: %v", err)
	}
	if cfgOverride.RateLimit.Auth.ResendVerificationRPM != 10 {
		t.Errorf("expected ResendVerificationRPM to be 10, got %d", cfgOverride.RateLimit.Auth.ResendVerificationRPM)
	}

	// 3. Invalid RPM <= 0 rejected
	os.Setenv("RATE_LIMIT_AUTH_RESEND_VERIFICATION_RPM", "0")
	_, err = LoadConfig()
	if err == nil {
		t.Fatal("expected error when RATE_LIMIT_AUTH_RESEND_VERIFICATION_RPM is 0")
	}
}
