package aws

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

// ACMAPI defines low-level ACM API interactions required by Client for testing flexibility.
type ACMAPI interface {
	RequestCertificate(ctx context.Context, params *acm.RequestCertificateInput, optFns ...func(*acm.Options)) (*acm.RequestCertificateOutput, error)
	DescribeCertificate(ctx context.Context, params *acm.DescribeCertificateInput, optFns ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error)
	DeleteCertificate(ctx context.Context, params *acm.DeleteCertificateInput, optFns ...func(*acm.Options)) (*acm.DeleteCertificateOutput, error)
}

// ALBAPI defines low-level ALB API interactions required by Client for testing flexibility.
type ALBAPI interface {
	AddListenerCertificates(ctx context.Context, params *elasticloadbalancingv2.AddListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.AddListenerCertificatesOutput, error)
	RemoveListenerCertificates(ctx context.Context, params *elasticloadbalancingv2.RemoveListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.RemoveListenerCertificatesOutput, error)
}

// Client wraps low-level AWS ACM and ELBv2 clients.
type Client struct {
	acmClient ACMAPI
	albClient ALBAPI
	cfg       *AWSConfig
}

// NewClient initializes AWS SDK clients using EC2 IAM Role / default credential chain.
func NewClient(ctx context.Context, region string, listenerARN string, enabled bool) (*Client, error) {
	validatedCfg, err := ValidateAWSConfig(region, listenerARN, enabled)
	if err != nil {
		return nil, fmt.Errorf("validate AWS config: %w", err)
	}

	if !validatedCfg.AutomationEnabled {
		return &Client{
			acmClient: nil,
			albClient: nil,
			cfg:       validatedCfg,
		}, nil
	}

	sdkCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(validatedCfg.Region))
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK config: %w", err)
	}

	return &Client{
		acmClient: acm.NewFromConfig(sdkCfg),
		albClient: elasticloadbalancingv2.NewFromConfig(sdkCfg),
		cfg:       validatedCfg,
	}, nil
}

// NewClientWithAPIs allows injection of mock low-level ACM and ALB SDK clients for unit testing.
func NewClientWithAPIs(acmAPI ACMAPI, albAPI ALBAPI, cfg *AWSConfig) *Client {
	return &Client{
		acmClient: acmAPI,
		albClient: albAPI,
		cfg:       cfg,
	}
}

// Config returns the validated AWS configuration.
func (c *Client) Config() *AWSConfig {
	return c.cfg
}
