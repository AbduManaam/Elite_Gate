package aws

import (
	"context"
	"fmt"
)

// MockAWSClient is a test double implementing CertificateManager and LoadBalancerManager.
type MockAWSClient struct {
	RequestCertificateFn            func(ctx context.Context, hostname string, idempotencyToken string) (string, error)
	DescribeCertificateFn           func(ctx context.Context, certificateARN string) (*CertificateDetails, error)
	DeleteCertificateFn             func(ctx context.Context, certificateARN string) error
	AttachCertificateToListenerFn   func(ctx context.Context, listenerARN string, certificateARN string) error
	DetachCertificateFromListenerFn func(ctx context.Context, listenerARN string, certificateARN string) error
	EnsureHostRuleFn                func(ctx context.Context, listenerARN string, hostname string, targetGroupARN string, minPriority int32, maxPriority int32) (string, int32, error)
	DeleteGatewayHostRuleFn         func(ctx context.Context, ruleARN string) error
}

func (m *MockAWSClient) RequestCertificate(ctx context.Context, hostname string, idempotencyToken string) (string, error) {
	if m.RequestCertificateFn != nil {
		return m.RequestCertificateFn(ctx, hostname, idempotencyToken)
	}
	return fmt.Sprintf("arn:aws:acm:ap-south-1:123456789012:certificate/mock-%s", hostname), nil
}

func (m *MockAWSClient) DescribeCertificate(ctx context.Context, certificateARN string) (*CertificateDetails, error) {
	if m.DescribeCertificateFn != nil {
		return m.DescribeCertificateFn(ctx, certificateARN)
	}
	return &CertificateDetails{
		ARN:             certificateARN,
		Status:          "ISSUED",
		ValidationName:  "_acm-challenge.example.com",
		ValidationValue: "_acm-value.aws",
	}, nil
}

func (m *MockAWSClient) DeleteCertificate(ctx context.Context, certificateARN string) error {
	if m.DeleteCertificateFn != nil {
		return m.DeleteCertificateFn(ctx, certificateARN)
	}
	return nil
}

func (m *MockAWSClient) AttachCertificateToListener(ctx context.Context, listenerARN string, certificateARN string) error {
	if m.AttachCertificateToListenerFn != nil {
		return m.AttachCertificateToListenerFn(ctx, listenerARN, certificateARN)
	}
	return nil
}

func (m *MockAWSClient) DetachCertificateFromListener(ctx context.Context, listenerARN string, certificateARN string) error {
	if m.DetachCertificateFromListenerFn != nil {
		return m.DetachCertificateFromListenerFn(ctx, listenerARN, certificateARN)
	}
	return nil
}

func (m *MockAWSClient) EnsureHostRule(
	ctx context.Context,
	listenerARN string,
	hostname string,
	targetGroupARN string,
	minPriority int32,
	maxPriority int32,
) (string, int32, error) {
	if m.EnsureHostRuleFn != nil {
		return m.EnsureHostRuleFn(ctx, listenerARN, hostname, targetGroupARN, minPriority, maxPriority)
	}
	return fmt.Sprintf("arn:aws:elasticloadbalancing:ap-south-1:123456789012:listener-rule/app/mock/%s", hostname), minPriority, nil
}

func (m *MockAWSClient) DeleteGatewayHostRule(ctx context.Context, ruleARN string) error {
	if m.DeleteGatewayHostRuleFn != nil {
		return m.DeleteGatewayHostRuleFn(ctx, ruleARN)
	}
	return nil
}
