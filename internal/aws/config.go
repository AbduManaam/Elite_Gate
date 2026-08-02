package aws

import (
	"errors"
	"strings"
)

// AWSConfig holds AWS region, listener ARN, and automation flag settings.
type AWSConfig struct {
	Region              string
	ALBHTTPSListenerARN string
	AutomationEnabled   bool
}

// ValidateAWSConfig verifies AWS configuration values based on whether automation is enabled.
func ValidateAWSConfig(region string, listenerARN string, enabled bool) (*AWSConfig, error) {
	if !enabled {
		return &AWSConfig{
			Region:              strings.TrimSpace(region),
			ALBHTTPSListenerARN: strings.TrimSpace(listenerARN),
			AutomationEnabled:   false,
		}, nil
	}

	trimmedRegion := strings.TrimSpace(region)
	if trimmedRegion == "" {
		return nil, errors.New("AWS_REGION is required when automation is enabled")
	}

	trimmedListener := strings.TrimSpace(listenerARN)
	if trimmedListener == "" {
		return nil, errors.New("ALB_HTTPS_LISTENER_ARN is required when automation is enabled")
	}

	return &AWSConfig{
		Region:              trimmedRegion,
		ALBHTTPSListenerARN: trimmedListener,
		AutomationEnabled:   true,
	}, nil
}
