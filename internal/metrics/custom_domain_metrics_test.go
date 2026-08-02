package metrics

import (
	"errors"
	"testing"

	acmTypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	elbv2Types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeFailureReason_TypedAndStringErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: ReasonUnknown,
		},
		{
			name:     "ACM AccessDeniedException",
			err:      &acmTypes.AccessDeniedException{},
			expected: ReasonAccessDenied,
		},
		{
			name:     "ACM InvalidParameterException",
			err:      &acmTypes.InvalidParameterException{},
			expected: ReasonInvalidParameter,
		},
		{
			name:     "ACM ResourceNotFoundException",
			err:      &acmTypes.ResourceNotFoundException{},
			expected: ReasonResourceNotFound,
		},
		{
			name:     "ELBv2 ListenerNotFoundException",
			err:      &elbv2Types.ListenerNotFoundException{},
			expected: ReasonResourceNotFound,
		},
		{
			name:     "ELBv2 TooManyCertificatesException",
			err:      &elbv2Types.TooManyCertificatesException{},
			expected: ReasonQuotaExceeded,
		},
		{
			name:     "String fallback - access denied",
			err:      errors.New("IAM access denied for operation"),
			expected: ReasonAccessDenied,
		},
		{
			name:     "String fallback - default listener certificate",
			err:      errors.New("cannot remove default certificate"),
			expected: ReasonDefaultCertRejection,
		},
		{
			name:     "String fallback - unknown error",
			err:      errors.New("something weird happened"),
			expected: ReasonUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := SanitizeFailureReason(tt.err)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestNewCustomDomainMetrics_IsolatedRegistry(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewCustomDomainMetrics(registry)
	assert.NotNil(t, m)
	assert.Equal(t, registry, m.Registry)

	// Verify increment operations don't panic
	m.JobsClaimed.WithLabelValues(OpProvision).Inc()
	m.JobsCompleted.WithLabelValues(OpProvision).Inc()
	m.JobsFailed.WithLabelValues(OpProvision, ReasonAccessDenied).Inc()
	m.JobsRetried.WithLabelValues(OpProvision, CategoryTransient).Inc()
	m.JobsActive.WithLabelValues(OpProvision).Set(1)
	m.WorkerHeartbeatTimestamp.Set(1234567890)
}
