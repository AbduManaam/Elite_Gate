package aws_test

import (
	"context"
	"testing"

	"elitegate/internal/aws"

	awsSDK "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmTypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSDKACM implements aws.ACMAPI for SDK unit testing without network or credentials
type MockSDKACM struct {
	RequestCertFn  func(ctx context.Context, params *acm.RequestCertificateInput, optFns ...func(*acm.Options)) (*acm.RequestCertificateOutput, error)
	DescribeCertFn func(ctx context.Context, params *acm.DescribeCertificateInput, optFns ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error)
	DeleteCertFn   func(ctx context.Context, params *acm.DeleteCertificateInput, optFns ...func(*acm.Options)) (*acm.DeleteCertificateOutput, error)
}

func (m *MockSDKACM) RequestCertificate(ctx context.Context, params *acm.RequestCertificateInput, optFns ...func(*acm.Options)) (*acm.RequestCertificateOutput, error) {
	if m.RequestCertFn != nil {
		return m.RequestCertFn(ctx, params, optFns...)
	}
	return &acm.RequestCertificateOutput{
		CertificateArn: awsSDK.String("arn:aws:acm:ap-south-1:123456789012:certificate/test-id"),
	}, nil
}

func (m *MockSDKACM) DescribeCertificate(ctx context.Context, params *acm.DescribeCertificateInput, optFns ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error) {
	if m.DescribeCertFn != nil {
		return m.DescribeCertFn(ctx, params, optFns...)
	}
	return &acm.DescribeCertificateOutput{
		Certificate: &acmTypes.CertificateDetail{
			CertificateArn: params.CertificateArn,
			Status:         acmTypes.CertificateStatusIssued,
		},
	}, nil
}

func (m *MockSDKACM) DeleteCertificate(ctx context.Context, params *acm.DeleteCertificateInput, optFns ...func(*acm.Options)) (*acm.DeleteCertificateOutput, error) {
	if m.DeleteCertFn != nil {
		return m.DeleteCertFn(ctx, params, optFns...)
	}
	return &acm.DeleteCertificateOutput{}, nil
}

// MockSDKALB implements aws.ALBAPI for SDK unit testing
type MockSDKALB struct {
	AddCertFn    func(ctx context.Context, params *elasticloadbalancingv2.AddListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.AddListenerCertificatesOutput, error)
	RemoveCertFn func(ctx context.Context, params *elasticloadbalancingv2.RemoveListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.RemoveListenerCertificatesOutput, error)
}

func (m *MockSDKALB) AddListenerCertificates(ctx context.Context, params *elasticloadbalancingv2.AddListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.AddListenerCertificatesOutput, error) {
	if m.AddCertFn != nil {
		return m.AddCertFn(ctx, params, optFns...)
	}
	return &elasticloadbalancingv2.AddListenerCertificatesOutput{}, nil
}

func (m *MockSDKALB) RemoveListenerCertificates(ctx context.Context, params *elasticloadbalancingv2.RemoveListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.RemoveListenerCertificatesOutput, error) {
	if m.RemoveCertFn != nil {
		return m.RemoveCertFn(ctx, params, optFns...)
	}
	return &elasticloadbalancingv2.RemoveListenerCertificatesOutput{}, nil
}

func TestValidateAWSConfig(t *testing.T) {
	// 1. Automation disabled: Region & Listener ARN not required
	cfg, err := aws.ValidateAWSConfig("", "", false)
	require.NoError(t, err)
	assert.False(t, cfg.AutomationEnabled)

	// 2. Automation enabled: Missing region
	_, err = aws.ValidateAWSConfig("", "arn:aws:elasticloadbalancing:ap-south-1:123:listener/123", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS_REGION")

	// 3. Automation enabled: Missing listener ARN
	_, err = aws.ValidateAWSConfig("ap-south-1", "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ALB_HTTPS_LISTENER_ARN")

	// 4. Automation enabled: Valid config
	cfg, err = aws.ValidateAWSConfig("ap-south-1", "arn:aws:elasticloadbalancing:ap-south-1:123:listener/123", true)
	require.NoError(t, err)
	assert.True(t, cfg.AutomationEnabled)
	assert.Equal(t, "ap-south-1", cfg.Region)
	assert.Equal(t, "arn:aws:elasticloadbalancing:ap-south-1:123:listener/123", cfg.ALBHTTPSListenerARN)
}

func TestACMRequestCertificate(t *testing.T) {
	ctx := context.Background()
	mockACM := &MockSDKACM{
		RequestCertFn: func(ctx context.Context, params *acm.RequestCertificateInput, optFns ...func(*acm.Options)) (*acm.RequestCertificateOutput, error) {
			assert.Equal(t, "app.example.com", awsSDK.ToString(params.DomainName))
			assert.Equal(t, acmTypes.ValidationMethodDns, params.ValidationMethod)
			assert.Equal(t, "eg123", awsSDK.ToString(params.IdempotencyToken))
			return &acm.RequestCertificateOutput{
				CertificateArn: awsSDK.String("arn:aws:acm:ap-south-1:123:certificate/abc"),
			}, nil
		},
	}

	cfg, _ := aws.ValidateAWSConfig("ap-south-1", "arn:aws:listener", true)
	client := aws.NewClientWithAPIs(mockACM, &MockSDKALB{}, cfg)

	arn, err := client.RequestCertificate(ctx, "app.example.com", "eg123")
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:acm:ap-south-1:123:certificate/abc", arn)
}

func TestACMDescribeCertificate_DelayedValidationRecord(t *testing.T) {
	ctx := context.Background()

	// 1. ACM returning certificate details BEFORE DNS validation record is generated by ACM
	mockACMPreparing := &MockSDKACM{
		DescribeCertFn: func(ctx context.Context, params *acm.DescribeCertificateInput, optFns ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error) {
			return &acm.DescribeCertificateOutput{
				Certificate: &acmTypes.CertificateDetail{
					CertificateArn:          awsSDK.String("arn:aws:acm:ap-south-1:123:certificate/abc"),
					Status:                  acmTypes.CertificateStatusPendingValidation,
					DomainValidationOptions: []acmTypes.DomainValidation{}, // No validation options yet
				},
			}, nil
		},
	}

	cfg, _ := aws.ValidateAWSConfig("ap-south-1", "arn:aws:listener", true)
	clientPreparing := aws.NewClientWithAPIs(mockACMPreparing, &MockSDKALB{}, cfg)

	details, err := clientPreparing.DescribeCertificate(ctx, "arn:aws:acm:ap-south-1:123:certificate/abc")
	require.NoError(t, err)
	assert.Equal(t, "PENDING_VALIDATION", details.Status)
	assert.Empty(t, details.ValidationName)
	assert.Empty(t, details.ValidationValue)

	// 2. ACM returning certificate details WITH populated DNS validation CNAME record
	mockACMPopulated := &MockSDKACM{
		DescribeCertFn: func(ctx context.Context, params *acm.DescribeCertificateInput, optFns ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error) {
			return &acm.DescribeCertificateOutput{
				Certificate: &acmTypes.CertificateDetail{
					CertificateArn: awsSDK.String("arn:aws:acm:ap-south-1:123:certificate/abc"),
					Status:         acmTypes.CertificateStatusPendingValidation,
					DomainValidationOptions: []acmTypes.DomainValidation{
						{
							DomainName: awsSDK.String("app.example.com"),
							ResourceRecord: &acmTypes.ResourceRecord{
								Name:  awsSDK.String("_acm.app.example.com."),
								Type:  acmTypes.RecordTypeCname,
								Value: awsSDK.String("_acm-val.aws."),
							},
						},
					},
				},
			}, nil
		},
	}

	clientPopulated := aws.NewClientWithAPIs(mockACMPopulated, &MockSDKALB{}, cfg)
	detailsPopulated, err := clientPopulated.DescribeCertificate(ctx, "arn:aws:acm:ap-south-1:123:certificate/abc")
	require.NoError(t, err)
	assert.Equal(t, "PENDING_VALIDATION", detailsPopulated.Status)
	assert.Equal(t, "_acm.app.example.com.", detailsPopulated.ValidationName)
	assert.Equal(t, "_acm-val.aws.", detailsPopulated.ValidationValue)
}

func TestALBAttachAndDetach(t *testing.T) {
	ctx := context.Background()

	mockALB := &MockSDKALB{
		AddCertFn: func(ctx context.Context, params *elasticloadbalancingv2.AddListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.AddListenerCertificatesOutput, error) {
			assert.Equal(t, "arn:aws:listener", awsSDK.ToString(params.ListenerArn))
			assert.Len(t, params.Certificates, 1)
			assert.Equal(t, "arn:aws:cert", awsSDK.ToString(params.Certificates[0].CertificateArn))
			return &elasticloadbalancingv2.AddListenerCertificatesOutput{}, nil
		},
		RemoveCertFn: func(ctx context.Context, params *elasticloadbalancingv2.RemoveListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.RemoveListenerCertificatesOutput, error) {
			assert.Equal(t, "arn:aws:listener", awsSDK.ToString(params.ListenerArn))
			assert.Len(t, params.Certificates, 1)
			assert.Equal(t, "arn:aws:cert", awsSDK.ToString(params.Certificates[0].CertificateArn))
			return &elasticloadbalancingv2.RemoveListenerCertificatesOutput{}, nil
		},
	}

	cfg, _ := aws.ValidateAWSConfig("ap-south-1", "arn:aws:listener", true)
	client := aws.NewClientWithAPIs(&MockSDKACM{}, mockALB, cfg)

	err := client.AttachCertificateToListener(ctx, "arn:aws:listener", "arn:aws:cert")
	require.NoError(t, err)

	err = client.DetachCertificateFromListener(ctx, "arn:aws:listener", "arn:aws:cert")
	require.NoError(t, err)
}

func TestDisabledAutomation(t *testing.T) {
	ctx := context.Background()
	cfg, _ := aws.ValidateAWSConfig("ap-south-1", "arn:aws:listener", false)
	client := aws.NewClientWithAPIs(&MockSDKACM{}, &MockSDKALB{}, cfg)

	_, err := client.RequestCertificate(ctx, "app.example.com", "token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")

	_, err = client.DescribeCertificate(ctx, "arn:aws:cert")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")

	err = client.AttachCertificateToListener(ctx, "arn:aws:listener", "arn:aws:cert")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

func TestMockAWSClient(t *testing.T) {
	ctx := context.Background()
	var client aws.AWSProvisioner = &aws.MockAWSClient{}

	certARN, err := client.RequestCertificate(ctx, "test.com", "tok")
	require.NoError(t, err)
	assert.Contains(t, certARN, "mock-test.com")

	details, err := client.DescribeCertificate(ctx, certARN)
	require.NoError(t, err)
	assert.Equal(t, "ISSUED", details.Status)

	err = client.AttachCertificateToListener(ctx, "arn:listener", certARN)
	require.NoError(t, err)

	err = client.DetachCertificateFromListener(ctx, "arn:listener", certARN)
	require.NoError(t, err)

	err = client.DeleteCertificate(ctx, certARN)
	require.NoError(t, err)
}
