package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	albTypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

// AttachCertificateToListener adds an ACM certificate to an ALB HTTPS listener.
func (c *Client) AttachCertificateToListener(ctx context.Context, listenerARN string, certificateARN string) error {
	if !c.cfg.AutomationEnabled {
		return errors.New("AWS custom domain automation is disabled")
	}
	if c.albClient == nil {
		return errors.New("ALB client is not initialized")
	}

	input := &elasticloadbalancingv2.AddListenerCertificatesInput{
		ListenerArn: aws.String(listenerARN),
		Certificates: []albTypes.Certificate{
			{CertificateArn: aws.String(certificateARN)},
		},
	}

	_, err := c.albClient.AddListenerCertificates(ctx, input)
	if err != nil {
		if isAlreadyExistsError(err) {
			return nil // Idempotent: certificate already attached
		}
		return fmt.Errorf("alb attach certificate %s to listener %s: %w", certificateARN, listenerARN, err)
	}

	return nil
}

// DetachCertificateFromListener removes an ACM certificate from an ALB HTTPS listener.
func (c *Client) DetachCertificateFromListener(ctx context.Context, listenerARN string, certificateARN string) error {
	if !c.cfg.AutomationEnabled {
		return errors.New("AWS custom domain automation is disabled")
	}
	if c.albClient == nil {
		return errors.New("ALB client is not initialized")
	}

	input := &elasticloadbalancingv2.RemoveListenerCertificatesInput{
		ListenerArn: aws.String(listenerARN),
		Certificates: []albTypes.Certificate{
			{CertificateArn: aws.String(certificateARN)},
		},
	}

	_, err := c.albClient.RemoveListenerCertificates(ctx, input)
	if err != nil {
		if isNotFoundError(err) || isAlreadyExistsError(err) {
			return nil // Idempotent: certificate already detached or not found
		}
		return fmt.Errorf("alb detach certificate %s from listener %s: %w", certificateARN, listenerARN, err)
	}

	return nil
}

func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "alreadyexists") || strings.Contains(msg, "already attached") || strings.Contains(msg, "already exists")
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "notfound") || strings.Contains(msg, "does not exist")
}
