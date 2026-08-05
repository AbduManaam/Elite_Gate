package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DedicatedGatewayAutomationConfig contains the AWS and hostname configuration
// required for exposing dedicated gateway containers through the production ALB.
type DedicatedGatewayAutomationConfig struct {
	Enabled     bool
	Region      string
	ListenerARN string
	VPCID       string
	InstanceID  string
	BaseDomain  string
	PriorityMin int
	PriorityMax int
}

func loadDedicatedGatewayAutomationConfig() (DedicatedGatewayAutomationConfig, error) {
	priorityMin, err := readDedicatedGatewayEnvInt(
		"DEDICATED_GATEWAY_RULE_PRIORITY_MIN",
		1000,
	)
	if err != nil {
		return DedicatedGatewayAutomationConfig{}, err
	}

	priorityMax, err := readDedicatedGatewayEnvInt(
		"DEDICATED_GATEWAY_RULE_PRIORITY_MAX",
		40000,
	)
	if err != nil {
		return DedicatedGatewayAutomationConfig{}, err
	}

	cfg := DedicatedGatewayAutomationConfig{
		Enabled: strings.EqualFold(
			strings.TrimSpace(
				os.Getenv("DEDICATED_GATEWAY_AWS_AUTOMATION_ENABLED"),
			),
			"true",
		),
		Region: strings.TrimSpace(
			os.Getenv("AWS_REGION"),
		),
		ListenerARN: strings.TrimSpace(
			os.Getenv("ALB_HTTPS_LISTENER_ARN"),
		),
		VPCID: strings.TrimSpace(
			os.Getenv("AWS_VPC_ID"),
		),
		InstanceID: strings.TrimSpace(
			os.Getenv("AWS_EC2_INSTANCE_ID"),
		),
		BaseDomain: strings.TrimSuffix(
			strings.TrimSpace(
				os.Getenv("DEDICATED_GATEWAY_BASE_DOMAIN"),
			),
			".",
		),
		PriorityMin: priorityMin,
		PriorityMax: priorityMax,
	}

	if !cfg.Enabled {
		return cfg, nil
	}

	required := map[string]string{
		"AWS_REGION":                    cfg.Region,
		"ALB_HTTPS_LISTENER_ARN":        cfg.ListenerARN,
		"AWS_VPC_ID":                    cfg.VPCID,
		"AWS_EC2_INSTANCE_ID":           cfg.InstanceID,
		"DEDICATED_GATEWAY_BASE_DOMAIN": cfg.BaseDomain,
	}

	for name, value := range required {
		if value == "" {
			return DedicatedGatewayAutomationConfig{},
				fmt.Errorf(
					"%s is required when dedicated gateway automation is enabled",
					name,
				)
		}
	}

	if strings.Contains(cfg.BaseDomain, "://") {
		return DedicatedGatewayAutomationConfig{},
			errors.New(
				"DEDICATED_GATEWAY_BASE_DOMAIN must contain only the domain name",
			)
	}

	if strings.ContainsAny(cfg.BaseDomain, "/ \t\r\n") {
		return DedicatedGatewayAutomationConfig{},
			errors.New(
				"DEDICATED_GATEWAY_BASE_DOMAIN must be a valid domain name without paths or whitespace",
			)
	}

	if cfg.PriorityMin < 1 ||
		cfg.PriorityMax > 50000 ||
		cfg.PriorityMin > cfg.PriorityMax {
		return DedicatedGatewayAutomationConfig{},
			errors.New(
				"dedicated gateway listener priority range must be between 1 and 50000",
			)
	}

	return cfg, nil
}

func readDedicatedGatewayEnvInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf(
			"%s must be a valid integer: %w",
			name,
			err,
		)
	}

	return number, nil
}
