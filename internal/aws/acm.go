package aws

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmTypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
)

// RequestCertificate requests a new DNS-validated ACM certificate for the hostname.
func (c *Client) RequestCertificate(ctx context.Context, hostname string, idempotencyToken string) (string, error) {
	if !c.cfg.AutomationEnabled {
		return "", errors.New("AWS custom domain automation is disabled")
	}
	if c.acmClient == nil {
		return "", errors.New("ACM client is not initialized")
	}

	input := &acm.RequestCertificateInput{
		DomainName:       aws.String(hostname),
		ValidationMethod: acmTypes.ValidationMethodDns,
		IdempotencyToken: aws.String(idempotencyToken),
	}

	output, err := c.acmClient.RequestCertificate(ctx, input)
	if err != nil {
		return "", fmt.Errorf("acm request certificate for %s: %w", hostname, err)
	}

	if output == nil || output.CertificateArn == nil {
		return "", fmt.Errorf("acm request certificate for %s returned empty ARN", hostname)
	}

	return aws.ToString(output.CertificateArn), nil
}

// DescribeCertificate safely inspects an ACM certificate and extracts DNS validation record details.
func (c *Client) DescribeCertificate(ctx context.Context, certificateARN string) (*CertificateDetails, error) {
	if !c.cfg.AutomationEnabled {
		return nil, errors.New("AWS custom domain automation is disabled")
	}
	if c.acmClient == nil {
		return nil, errors.New("ACM client is not initialized")
	}

	input := &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certificateARN),
	}

	output, err := c.acmClient.DescribeCertificate(ctx, input)
	if err != nil {
		var notFoundErr *acmTypes.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return nil, fmt.Errorf("acm certificate %s not found: %w", certificateARN, err)
		}
		return nil, fmt.Errorf("acm describe certificate %s: %w", certificateARN, err)
	}

	if output == nil || output.Certificate == nil {
		return nil, fmt.Errorf("acm describe certificate %s returned empty response", certificateARN)
	}

	cert := output.Certificate
	details := &CertificateDetails{
		ARN:      aws.ToString(cert.CertificateArn),
		Status:   string(cert.Status),
		IssuedAt: cert.IssuedAt,
	}

	if cert.FailureReason != "" {
		details.FailureReason = string(cert.FailureReason)
	}

	// Safely handle missing DomainValidationOptions or missing ResourceRecord while ACM is preparing them
	if len(cert.DomainValidationOptions) > 0 {
		opt := cert.DomainValidationOptions[0]
		if opt.ResourceRecord != nil {
			details.ValidationName = aws.ToString(opt.ResourceRecord.Name)
			details.ValidationValue = aws.ToString(opt.ResourceRecord.Value)
		}
	}

	return details, nil
}

// DeleteCertificate deletes an ACM certificate.
func (c *Client) DeleteCertificate(ctx context.Context, certificateARN string) error {
	if !c.cfg.AutomationEnabled {
		return errors.New("AWS custom domain automation is disabled")
	}
	if c.acmClient == nil {
		return errors.New("ACM client is not initialized")
	}

	input := &acm.DeleteCertificateInput{
		CertificateArn: aws.String(certificateARN),
	}

	_, err := c.acmClient.DeleteCertificate(ctx, input)
	if err != nil {
		var notFoundErr *acmTypes.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return nil // Idempotent handling if already deleted
		}
		return fmt.Errorf("acm delete certificate %s: %w", certificateARN, err)
	}

	return nil
}
