package main

import (
	"testing"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DEDICATED_GATEWAY_AWS_AUTOMATION_ENABLED", "true")
	t.Setenv("AWS_REGION", "ap-south-1")
	t.Setenv("ALB_HTTPS_LISTENER_ARN", "arn:aws:elasticloadbalancing:ap-south-1:123456789012:listener/app/my-alb/123/456")
	t.Setenv("AWS_VPC_ID", "vpc-12345678")
	t.Setenv("AWS_EC2_INSTANCE_ID", "i-1234567890abcdef0")
	t.Setenv("DEDICATED_GATEWAY_BASE_DOMAIN", "elitegateway.site")
	t.Setenv("DEDICATED_GATEWAY_RULE_PRIORITY_MIN", "1000")
	t.Setenv("DEDICATED_GATEWAY_RULE_PRIORITY_MAX", "40000")
}

func TestLoadDedicatedGatewayAutomationConfig_DisabledWithMissingAWSConfig(t *testing.T) {
	t.Setenv("DEDICATED_GATEWAY_AWS_AUTOMATION_ENABLED", "false")
	t.Setenv("AWS_REGION", "")

	cfg, err := loadDedicatedGatewayAutomationConfig()
	if err != nil {
		t.Fatalf("expected success when disabled, got error: %v", err)
	}
	if cfg.Enabled {
		t.Errorf("expected Enabled to be false, got true")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_EnabledWithValidValues(t *testing.T) {
	setValidEnv(t)

	cfg, err := loadDedicatedGatewayAutomationConfig()
	if err != nil {
		t.Fatalf("expected success with valid values, got error: %v", err)
	}
	if !cfg.Enabled {
		t.Errorf("expected Enabled to be true")
	}
	if cfg.Region != "ap-south-1" {
		t.Errorf("expected Region ap-south-1, got %s", cfg.Region)
	}
	if cfg.BaseDomain != "elitegateway.site" {
		t.Errorf("expected BaseDomain elitegateway.site, got %s", cfg.BaseDomain)
	}
	if cfg.PriorityMin != 1000 || cfg.PriorityMax != 40000 {
		t.Errorf("expected priority range 1000-40000, got %d-%d", cfg.PriorityMin, cfg.PriorityMax)
	}
}

func TestLoadDedicatedGatewayAutomationConfig_MissingRegionFails(t *testing.T) {
	setValidEnv(t)
	t.Setenv("AWS_REGION", "")

	_, err := loadDedicatedGatewayAutomationConfig()
	if err == nil {
		t.Fatal("expected error for missing region, got nil")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_MissingListenerARNFails(t *testing.T) {
	setValidEnv(t)
	t.Setenv("ALB_HTTPS_LISTENER_ARN", "")

	_, err := loadDedicatedGatewayAutomationConfig()
	if err == nil {
		t.Fatal("expected error for missing listener ARN, got nil")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_MissingVPCIDFails(t *testing.T) {
	setValidEnv(t)
	t.Setenv("AWS_VPC_ID", "")

	_, err := loadDedicatedGatewayAutomationConfig()
	if err == nil {
		t.Fatal("expected error for missing VPC ID, got nil")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_MissingEC2InstanceIDFails(t *testing.T) {
	setValidEnv(t)
	t.Setenv("AWS_EC2_INSTANCE_ID", "")

	_, err := loadDedicatedGatewayAutomationConfig()
	if err == nil {
		t.Fatal("expected error for missing EC2 instance ID, got nil")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_MissingBaseDomainFails(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DEDICATED_GATEWAY_BASE_DOMAIN", "")

	_, err := loadDedicatedGatewayAutomationConfig()
	if err == nil {
		t.Fatal("expected error for missing base domain, got nil")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_BaseDomainWithSchemeFails(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DEDICATED_GATEWAY_BASE_DOMAIN", "https://elitegateway.site")

	_, err := loadDedicatedGatewayAutomationConfig()
	if err == nil {
		t.Fatal("expected error for base domain with https://, got nil")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_InvalidPriorityIntegerFails(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DEDICATED_GATEWAY_RULE_PRIORITY_MIN", "invalid")

	_, err := loadDedicatedGatewayAutomationConfig()
	if err == nil {
		t.Fatal("expected error for invalid priority integer, got nil")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_PriorityMinLessThan1Fails(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DEDICATED_GATEWAY_RULE_PRIORITY_MIN", "0")

	_, err := loadDedicatedGatewayAutomationConfig()
	if err == nil {
		t.Fatal("expected error for priority min < 1, got nil")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_PriorityMaxGreaterThan50000Fails(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DEDICATED_GATEWAY_RULE_PRIORITY_MAX", "50001")

	_, err := loadDedicatedGatewayAutomationConfig()
	if err == nil {
		t.Fatal("expected error for priority max > 50000, got nil")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_MinGreaterThanMaxFails(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DEDICATED_GATEWAY_RULE_PRIORITY_MIN", "5000")
	t.Setenv("DEDICATED_GATEWAY_RULE_PRIORITY_MAX", "1000")

	_, err := loadDedicatedGatewayAutomationConfig()
	if err == nil {
		t.Fatal("expected error when priority min > priority max, got nil")
	}
}

func TestLoadDedicatedGatewayAutomationConfig_TrailingDotInBaseDomainRemoved(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DEDICATED_GATEWAY_BASE_DOMAIN", "elitegateway.site.")

	cfg, err := loadDedicatedGatewayAutomationConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseDomain != "elitegateway.site" {
		t.Errorf("expected elitegateway.site, got %s", cfg.BaseDomain)
	}
}

func TestLoadDedicatedGatewayAutomationConfig_WhitespaceTrimmed(t *testing.T) {
	setValidEnv(t)
	t.Setenv("AWS_REGION", "  ap-south-1  ")
	t.Setenv("DEDICATED_GATEWAY_BASE_DOMAIN", "  elitegateway.site  ")

	cfg, err := loadDedicatedGatewayAutomationConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Region != "ap-south-1" {
		t.Errorf("expected trimmed region, got %q", cfg.Region)
	}
	if cfg.BaseDomain != "elitegateway.site" {
		t.Errorf("expected trimmed base domain, got %q", cfg.BaseDomain)
	}
}
