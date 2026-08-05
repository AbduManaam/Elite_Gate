package aws

import (
	"context"
	"time"
)

// CertificateDetails contains normalized ACM certificate state and validation record information.
type CertificateDetails struct {
	ARN             string
	Status          string
	ValidationName  string
	ValidationValue string
	FailureReason   string
	IssuedAt        *time.Time
}

// CertificateManager defines AWS ACM operations for certificate request, describe, and deletion.
type CertificateManager interface {
	RequestCertificate(ctx context.Context, hostname string, idempotencyToken string) (certificateARN string, err error)
	DescribeCertificate(ctx context.Context, certificateARN string) (*CertificateDetails, error)
	DeleteCertificate(ctx context.Context, certificateARN string) error
}

// LoadBalancerManager defines AWS ALB listener certificate attachment and detachment operations.
type LoadBalancerManager interface {
	AttachCertificateToListener(ctx context.Context, listenerARN string, certificateARN string) error
	DetachCertificateFromListener(ctx context.Context, listenerARN string, certificateARN string) error
}

// AWSProvisioner combines ACM and ALB management interfaces for custom domain automation.
type AWSProvisioner interface {
	CertificateManager
	LoadBalancerManager
}

// DedicatedGatewayLoadBalancerManager defines ALB operations required for
// exposing and removing dedicated gateway containers.
type DedicatedGatewayLoadBalancerManager interface {
	CreateGatewayTargetGroup(
		ctx context.Context,
		name string,
		vpcID string,
	) (targetGroupARN string, err error)

	RegisterGatewayTarget(
		ctx context.Context,
		targetGroupARN string,
		instanceID string,
		port int32,
	) error

	GetGatewayTargetHealth(
		ctx context.Context,
		targetGroupARN string,
		instanceID string,
		port int32,
	) (state string, err error)

	CreateGatewayHostRule(
		ctx context.Context,
		listenerARN string,
		hostname string,
		targetGroupARN string,
		priority int32,
	) (ruleARN string, err error)

	DeleteGatewayHostRule(
		ctx context.Context,
		ruleARN string,
	) error

	DeregisterGatewayTarget(
		ctx context.Context,
		targetGroupARN string,
		instanceID string,
		port int32,
	) error

	DeleteGatewayTargetGroup(
		ctx context.Context,
		targetGroupARN string,
	) error
}
