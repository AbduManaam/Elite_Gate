package main

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"elitegate/internal/aws"
	"elitegate/internal/domain"
	"elitegate/internal/storage"

	acmTypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvisioningRepo struct {
	ClaimNextProvisioningJobFn  func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error)
	AdvanceProvisioningStateFn  func(ctx context.Context, params storage.AdvanceProvisioningParams) error
	ScheduleProvisioningPollFn  func(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, expectedStatus string, nextPollAt time.Time) error
	ScheduleProvisioningRetryFn func(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, expectedStatus string, nextRetryAt time.Time, provisioningError string) error
	MarkProvisioningFailedFn    func(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, expectedStatus string, provisioningError string) error
	MarkProvisioningCompletedFn func(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID) error
	MarkDeprovisionedFn         func(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID) error
	MarkDeprovisionFailedFn     func(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, errStr string) error
	ReleaseProvisioningLeaseFn  func(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID) error
}

func (m *mockProvisioningRepo) MarkDeprovisioned(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID) error {
	if m.MarkDeprovisionedFn != nil {
		return m.MarkDeprovisionedFn(ctx, id, leaseToken)
	}
	return nil
}

func (m *mockProvisioningRepo) MarkDeprovisionFailed(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, errStr string) error {
	if m.MarkDeprovisionFailedFn != nil {
		return m.MarkDeprovisionFailedFn(ctx, id, leaseToken, errStr)
	}
	return nil
}

func (m *mockProvisioningRepo) ClaimNextProvisioningJob(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
	if m.ClaimNextProvisioningJobFn != nil {
		return m.ClaimNextProvisioningJobFn(ctx, workerID, lockTimeout)
	}
	return nil, nil
}

func (m *mockProvisioningRepo) AdvanceProvisioningState(ctx context.Context, params storage.AdvanceProvisioningParams) error {
	if m.AdvanceProvisioningStateFn != nil {
		return m.AdvanceProvisioningStateFn(ctx, params)
	}
	return nil
}

func (m *mockProvisioningRepo) ScheduleProvisioningPoll(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, expectedStatus string, nextPollAt time.Time) error {
	if m.ScheduleProvisioningPollFn != nil {
		return m.ScheduleProvisioningPollFn(ctx, id, leaseToken, expectedStatus, nextPollAt)
	}
	return nil
}

func (m *mockProvisioningRepo) ScheduleProvisioningRetry(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, expectedStatus string, nextRetryAt time.Time, provisioningError string) error {
	if m.ScheduleProvisioningRetryFn != nil {
		return m.ScheduleProvisioningRetryFn(ctx, id, leaseToken, expectedStatus, nextRetryAt, provisioningError)
	}
	return nil
}

func (m *mockProvisioningRepo) MarkProvisioningFailed(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, expectedStatus string, provisioningError string) error {
	if m.MarkProvisioningFailedFn != nil {
		return m.MarkProvisioningFailedFn(ctx, id, leaseToken, expectedStatus, provisioningError)
	}
	return nil
}

func (m *mockProvisioningRepo) MarkProvisioningCompleted(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID) error {
	if m.MarkProvisioningCompletedFn != nil {
		return m.MarkProvisioningCompletedFn(ctx, id, leaseToken)
	}
	return nil
}

func (m *mockProvisioningRepo) ReleaseProvisioningLease(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID) error {
	if m.ReleaseProvisioningLeaseFn != nil {
		return m.ReleaseProvisioningLeaseFn(ctx, id, leaseToken)
	}
	return nil
}

func newTestProvisioner(repo ProvisioningRepository, mockAWS *aws.MockAWSClient) *Provisioner {
	return NewProvisioner(
		repo,
		mockAWS,
		mockAWS,
		"test-worker",
		"arn:aws:elasticloadbalancing:ap-south-1:123456789012:listener/app/alb/123/456",
		zerolog.Nop(),
	)
}

func TestGenerateACMIdempotencyToken(t *testing.T) {
	domainID1 := uuid.New()
	domainID2 := uuid.New()

	token1a := GenerateACMIdempotencyToken(domainID1)
	token1b := GenerateACMIdempotencyToken(domainID1)
	token2 := GenerateACMIdempotencyToken(domainID2)

	assert.Equal(t, token1a, token1b, "token generation must be deterministic")
	assert.NotEqual(t, token1a, token2, "different domain IDs must produce different tokens")

	assert.LessOrEqual(t, len(token1a), 32, "ACM token must not exceed 32 characters")

	matched, err := regexp.MatchString(`^eg[a-f0-9]{30}$`, token1a)
	require.NoError(t, err)
	assert.True(t, matched, "token must contain only valid prefix and hex characters")
}

func TestCalculateRetryDelay(t *testing.T) {
	assert.Equal(t, 15*time.Second, CalculateRetryDelay(0))
	assert.Equal(t, 15*time.Second, CalculateRetryDelay(1))
	assert.Equal(t, 30*time.Second, CalculateRetryDelay(2))
	assert.Equal(t, 1*time.Minute, CalculateRetryDelay(3))
	assert.Equal(t, 2*time.Minute, CalculateRetryDelay(4))
	assert.Equal(t, 5*time.Minute, CalculateRetryDelay(5))
	assert.Equal(t, 5*time.Minute, CalculateRetryDelay(10))
}

func TestRequestingCertificate_Success(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	mockCertARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	requestCalled := false
	mockAWS := &aws.MockAWSClient{
		RequestCertificateFn: func(ctx context.Context, hostname string, idempotencyToken string) (string, error) {
			requestCalled = true
			assert.Equal(t, "app.example.com", hostname)
			return mockCertARN, nil
		},
	}

	advanceCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                 jobID,
				Hostname:           "app.example.com",
				ProvisioningStatus: domain.ProvisioningStatusRequestingCertificate,
				LeaseToken:         &leaseToken,
			}, nil
		},
		AdvanceProvisioningStateFn: func(ctx context.Context, params storage.AdvanceProvisioningParams) error {
			advanceCalled = true
			assert.Equal(t, jobID, params.ID)
			assert.Equal(t, leaseToken, params.LeaseToken)
			assert.Equal(t, domain.ProvisioningStatusRequestingCertificate, params.ExpectedStatus)
			assert.Equal(t, domain.ProvisioningStatusWaitingForValidationRecord, params.NewStatus)
			require.NotNil(t, params.CertificateARN)
			assert.Equal(t, mockCertARN, *params.CertificateARN)
			require.NotNil(t, params.CertificateManagedByEliteGate, "CertificateManagedByEliteGate must be set when requesting certificate")
			assert.True(t, *params.CertificateManagedByEliteGate, "CertificateManagedByEliteGate must be true for requested ACM certificate")
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, requestCalled)
	assert.True(t, advanceCalled)
}

func TestRequestingCertificate_ExistingARN_SkipsRequest(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	existingARN := "arn:aws:acm:ap-south-1:123456789012:certificate/existing-cert"

	requestCalled := false
	mockAWS := &aws.MockAWSClient{
		RequestCertificateFn: func(ctx context.Context, hostname string, idempotencyToken string) (string, error) {
			requestCalled = true
			return "", errors.New("should not be called")
		},
	}

	advanceCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                 jobID,
				Hostname:           "app.example.com",
				ProvisioningStatus: domain.ProvisioningStatusRequestingCertificate,
				CertificateARN:     &existingARN,
				LeaseToken:         &leaseToken,
			}, nil
		},
		AdvanceProvisioningStateFn: func(ctx context.Context, params storage.AdvanceProvisioningParams) error {
			advanceCalled = true
			assert.Equal(t, existingARN, *params.CertificateARN)
			assert.Equal(t, domain.ProvisioningStatusWaitingForValidationRecord, params.NewStatus)
			require.NotNil(t, params.CertificateManagedByEliteGate, "CertificateManagedByEliteGate must be set for existing ARN path")
			assert.True(t, *params.CertificateManagedByEliteGate)
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.False(t, requestCalled, "RequestCertificate must be skipped when ARN exists")
	assert.True(t, advanceCalled)
}

func TestRequestingCertificate_TransientError_SchedulesRetry(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()

	mockAWS := &aws.MockAWSClient{
		RequestCertificateFn: func(ctx context.Context, hostname string, idempotencyToken string) (string, error) {
			return "", &acmTypes.ThrottlingException{Message: nil}
		},
	}

	retryScheduled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                   jobID,
				Hostname:             "app.example.com",
				ProvisioningStatus:   domain.ProvisioningStatusRequestingCertificate,
				ProvisioningAttempts: 1,
				LeaseToken:           &leaseToken,
			}, nil
		},
		ScheduleProvisioningRetryFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, nextRetryAt time.Time, provisioningError string) error {
			retryScheduled = true
			assert.Equal(t, jobID, id)
			assert.Equal(t, leaseToken, lToken)
			assert.Contains(t, provisioningError, "AWS ACM service transient error")
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, retryScheduled)
}

func TestRequestingCertificate_TerminalError_MarksFailed(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()

	mockAWS := &aws.MockAWSClient{
		RequestCertificateFn: func(ctx context.Context, hostname string, idempotencyToken string) (string, error) {
			return "", &acmTypes.InvalidParameterException{Message: nil}
		},
	}

	failedMarked := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                 jobID,
				Hostname:           "invalid domain syntax!!!",
				ProvisioningStatus: domain.ProvisioningStatusRequestingCertificate,
				LeaseToken:         &leaseToken,
			}, nil
		},
		MarkProvisioningFailedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, provisioningError string) error {
			failedMarked = true
			assert.Equal(t, jobID, id)
			assert.Equal(t, leaseToken, lToken)
			assert.Contains(t, provisioningError, "Invalid parameters")
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, failedMarked)
}

func TestWaitingForValidationRecord_RecordNotReady_SchedulesNormalPoll(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:             certARN,
				Status:          "PENDING_VALIDATION",
				ValidationName:  "",
				ValidationValue: "",
			}, nil
		},
	}

	pollScheduled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                 jobID,
				Hostname:           "app.example.com",
				ProvisioningStatus: domain.ProvisioningStatusWaitingForValidationRecord,
				CertificateARN:     &certARN,
				LeaseToken:         &leaseToken,
			}, nil
		},
		ScheduleProvisioningPollFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, nextPollAt time.Time) error {
			pollScheduled = true
			assert.Equal(t, jobID, id)
			assert.Equal(t, leaseToken, lToken)
			assert.Equal(t, domain.ProvisioningStatusWaitingForValidationRecord, expectedStatus)
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, pollScheduled)
}

func TestWaitingForValidationRecord_RecordAvailable_AdvancesState(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"
	valName := "_acm-challenge.app.example.com"
	valValue := "_acm-value.aws"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:             certARN,
				Status:          "PENDING_VALIDATION",
				ValidationName:  valName,
				ValidationValue: valValue,
			}, nil
		},
	}

	advanceCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                 jobID,
				Hostname:           "app.example.com",
				ProvisioningStatus: domain.ProvisioningStatusWaitingForValidationRecord,
				CertificateARN:     &certARN,
				LeaseToken:         &leaseToken,
			}, nil
		},
		AdvanceProvisioningStateFn: func(ctx context.Context, params storage.AdvanceProvisioningParams) error {
			advanceCalled = true
			assert.Equal(t, jobID, params.ID)
			assert.Equal(t, leaseToken, params.LeaseToken)
			assert.Equal(t, domain.ProvisioningStatusWaitingForValidationRecord, params.ExpectedStatus)
			assert.Equal(t, domain.ProvisioningStatusWaitingForDNS, params.NewStatus)
			require.NotNil(t, params.CertificateValidationName)
			assert.Equal(t, valName, *params.CertificateValidationName)
			require.NotNil(t, params.CertificateValidationValue)
			assert.Equal(t, valValue, *params.CertificateValidationValue)
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, advanceCalled)
}

func TestWaitingForValidationRecord_ACMFailedState_MarksFailed(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:           certARN,
				Status:        "FAILED",
				FailureReason: "DOMAIN_VALIDATION_TIMED_OUT",
			}, nil
		},
	}

	markFailedCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                 jobID,
				Hostname:           "app.example.com",
				ProvisioningStatus: domain.ProvisioningStatusWaitingForValidationRecord,
				CertificateARN:     &certARN,
				LeaseToken:         &leaseToken,
			}, nil
		},
		MarkProvisioningFailedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, provisioningError string) error {
			markFailedCalled = true
			assert.Equal(t, jobID, id)
			assert.Equal(t, leaseToken, lToken)
			assert.Contains(t, provisioningError, "DOMAIN_VALIDATION_TIMED_OUT")
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, markFailedCalled)
}

func TestProcessNextJob_NoJob_ReturnsFalse(t *testing.T) {
	mockAWS := &aws.MockAWSClient{}
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return nil, nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.False(t, processed, "must return false when no job is available")
}

func TestWaitingForDNS_PendingValidation_SchedulesPoll(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:    certARN,
				Status: "PENDING_VALIDATION",
			}, nil
		},
	}

	pollScheduled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                 jobID,
				Hostname:           "app.example.com",
				ProvisioningStatus: domain.ProvisioningStatusWaitingForDNS,
				CertificateARN:     &certARN,
				LeaseToken:         &leaseToken,
			}, nil
		},
		ScheduleProvisioningPollFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, nextPollAt time.Time) error {
			pollScheduled = true
			assert.Equal(t, jobID, id)
			assert.Equal(t, leaseToken, lToken)
			assert.Equal(t, domain.ProvisioningStatusWaitingForDNS, expectedStatus)
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, pollScheduled)
}

func TestWaitingForDNS_Issued_AdvancesStateToAttachingCertificate(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"
	now := time.Now().UTC()

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:      certARN,
				Status:   "ISSUED",
				IssuedAt: &now,
			}, nil
		},
	}

	advanceCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                 jobID,
				Hostname:           "app.example.com",
				ProvisioningStatus: domain.ProvisioningStatusWaitingForDNS,
				CertificateARN:     &certARN,
				LeaseToken:         &leaseToken,
			}, nil
		},
		AdvanceProvisioningStateFn: func(ctx context.Context, params storage.AdvanceProvisioningParams) error {
			advanceCalled = true
			assert.Equal(t, jobID, params.ID)
			assert.Equal(t, leaseToken, params.LeaseToken)
			assert.Equal(t, domain.ProvisioningStatusWaitingForDNS, params.ExpectedStatus)
			assert.Equal(t, domain.ProvisioningStatusAttachingCertificate, params.NewStatus)
			require.NotNil(t, params.CertificateStatus)
			assert.Equal(t, "issued", *params.CertificateStatus)
			require.NotNil(t, params.CertificateIssuedAt)
			assert.Equal(t, now, *params.CertificateIssuedAt)
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, advanceCalled)
}

func TestWaitingForDNS_TerminalFailures(t *testing.T) {
	terminalStatuses := []string{
		"FAILED",
		"VALIDATION_TIMED_OUT",
		"REVOKED",
		"EXPIRED",
		"INACTIVE",
	}

	for _, status := range terminalStatuses {
		t.Run(status, func(t *testing.T) {
			jobID := uuid.New()
			leaseToken := uuid.New()
			certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

			mockAWS := &aws.MockAWSClient{
				DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
					return &aws.CertificateDetails{
						ARN:           certARN,
						Status:        status,
						FailureReason: status,
					}, nil
				},
			}

			failedMarked := false
			mockRepo := &mockProvisioningRepo{
				ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
					return &domain.ProvisioningJob{
						ID:                 jobID,
						Hostname:           "app.example.com",
						ProvisioningStatus: domain.ProvisioningStatusWaitingForDNS,
						CertificateARN:     &certARN,
						LeaseToken:         &leaseToken,
					}, nil
				},
				MarkProvisioningFailedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, provisioningError string) error {
					failedMarked = true
					assert.Equal(t, jobID, id)
					assert.Equal(t, leaseToken, lToken)
					assert.Equal(t, domain.ProvisioningStatusWaitingForDNS, expectedStatus)
					assert.NotEmpty(t, provisioningError)
					return nil
				},
			}

			provisioner := newTestProvisioner(mockRepo, mockAWS)
			processed, err := provisioner.ProcessNextJob(context.Background())
			require.NoError(t, err)
			assert.True(t, processed)
			assert.True(t, failedMarked)
		})
	}
}

func TestWaitingForDNS_UnknownStatus_RetryAndMaxAttemptFailure(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:    certARN,
				Status: "UNKNOWN_CUSTOM_STATUS",
			}, nil
		},
	}

	// Case 1: Attempts < 5 -> retry scheduled
	retryScheduled := false
	mockRepoRetry := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                   jobID,
				Hostname:             "app.example.com",
				ProvisioningStatus:   domain.ProvisioningStatusWaitingForDNS,
				ProvisioningAttempts: 2,
				CertificateARN:       &certARN,
				LeaseToken:           &leaseToken,
			}, nil
		},
		ScheduleProvisioningRetryFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, nextRetryAt time.Time, provisioningError string) error {
			retryScheduled = true
			assert.Contains(t, provisioningError, "Unrecognized ACM status")
			return nil
		},
	}

	provisionerRetry := newTestProvisioner(mockRepoRetry, mockAWS)
	processed, err := provisionerRetry.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, retryScheduled)

	// Case 2: Attempts >= 5 -> marked failed
	failedMarked := false
	mockRepoFailed := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                   jobID,
				Hostname:             "app.example.com",
				ProvisioningStatus:   domain.ProvisioningStatusWaitingForDNS,
				ProvisioningAttempts: 5,
				CertificateARN:       &certARN,
				LeaseToken:           &leaseToken,
			}, nil
		},
		MarkProvisioningFailedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, provisioningError string) error {
			failedMarked = true
			assert.Contains(t, provisioningError, "Unknown ACM certificate status")
			return nil
		},
	}

	provisionerFailed := newTestProvisioner(mockRepoFailed, mockAWS)
	processed, err = provisionerFailed.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, failedMarked)
}

func TestWaitingForDNS_MissingARN_MarksFailed(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()

	mockAWS := &aws.MockAWSClient{}
	failedMarked := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                 jobID,
				Hostname:           "app.example.com",
				ProvisioningStatus: domain.ProvisioningStatusWaitingForDNS,
				CertificateARN:     nil,
				LeaseToken:         &leaseToken,
			}, nil
		},
		MarkProvisioningFailedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, provisioningError string) error {
			failedMarked = true
			assert.Contains(t, provisioningError, "Missing ACM certificate ARN")
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, failedMarked)
}

func TestWaitingForDNS_TransientDescribeError_SchedulesRetry(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return nil, errors.New("503 Service Unavailable")
		},
	}

	retryScheduled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                 jobID,
				Hostname:           "app.example.com",
				ProvisioningStatus: domain.ProvisioningStatusWaitingForDNS,
				CertificateARN:     &certARN,
				LeaseToken:         &leaseToken,
			}, nil
		},
		ScheduleProvisioningRetryFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, nextRetryAt time.Time, provisioningError string) error {
			retryScheduled = true
			assert.Contains(t, provisioningError, "AWS ACM service transient error")
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, retryScheduled)
}

func TestSanitizeACMFailureReason(t *testing.T) {
	assert.Equal(t, "ACM domain validation timed out", SanitizeACMFailureReason("DOMAIN_VALIDATION_TIMED_OUT"))
	assert.Equal(t, "ACM validation failed: no available contacts", SanitizeACMFailureReason("FAILURE_REASON_NO_AVAILABLE_CONTACTS"))
	assert.Equal(t, "ACM certificate was revoked", SanitizeACMFailureReason("REVOKED"))
	assert.Equal(t, "ACM certificate expired", SanitizeACMFailureReason("EXPIRED"))
	assert.Equal(t, "ACM certificate is inactive", SanitizeACMFailureReason("INACTIVE"))
	assert.Equal(t, "ACM certificate failed: CUSTOM_REASON", SanitizeACMFailureReason("CUSTOM_REASON"))
}

func TestAttachingCertificate_Success_MarksCompleted(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	attachCalled := false
	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:    certARN,
				Status: "ISSUED",
			}, nil
		},
		AttachCertificateToListenerFn: func(ctx context.Context, listenerARN string, certificateARN string) error {
			attachCalled = true
			assert.Equal(t, certARN, certificateARN)
			return nil
		},
	}

	completedCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                            jobID,
				Hostname:                      "app.example.com",
				ProvisioningStatus:            domain.ProvisioningStatusAttachingCertificate,
				CertificateARN:                &certARN,
				CertificateManagedByEliteGate: true,
				LeaseToken:                    &leaseToken,
			}, nil
		},
		MarkProvisioningCompletedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID) error {
			completedCalled = true
			assert.Equal(t, jobID, id)
			assert.Equal(t, leaseToken, lToken)
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, attachCalled)
	assert.True(t, completedCalled)
}

func TestAttachingCertificate_AlreadyAttached_TreatedAsSuccess(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:    certARN,
				Status: "ISSUED",
			}, nil
		},
		AttachCertificateToListenerFn: func(ctx context.Context, listenerARN string, certificateARN string) error {
			return nil // Already attached returns nil from AttachCertificateToListener
		},
	}

	completedCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                            jobID,
				Hostname:                      "app.example.com",
				ProvisioningStatus:            domain.ProvisioningStatusAttachingCertificate,
				CertificateARN:                &certARN,
				CertificateManagedByEliteGate: true,
				LeaseToken:                    &leaseToken,
			}, nil
		},
		MarkProvisioningCompletedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID) error {
			completedCalled = true
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, completedCalled)
}

func TestAttachingCertificate_PendingValidation_SchedulesPoll(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:    certARN,
				Status: "PENDING_VALIDATION",
			}, nil
		},
	}

	pollScheduled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                            jobID,
				Hostname:                      "app.example.com",
				ProvisioningStatus:            domain.ProvisioningStatusAttachingCertificate,
				CertificateARN:                &certARN,
				CertificateManagedByEliteGate: true,
				LeaseToken:                    &leaseToken,
			}, nil
		},
		ScheduleProvisioningPollFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, nextPollAt time.Time) error {
			pollScheduled = true
			assert.Equal(t, domain.ProvisioningStatusAttachingCertificate, expectedStatus)
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, pollScheduled)
}

func TestAttachingCertificate_FailedState_MarksFailed(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:           certARN,
				Status:        "FAILED",
				FailureReason: "DOMAIN_VALIDATION_TIMED_OUT",
			}, nil
		},
	}

	markFailedCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                            jobID,
				Hostname:                      "app.example.com",
				ProvisioningStatus:            domain.ProvisioningStatusAttachingCertificate,
				CertificateARN:                &certARN,
				CertificateManagedByEliteGate: true,
				LeaseToken:                    &leaseToken,
			}, nil
		},
		MarkProvisioningFailedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, provisioningError string) error {
			markFailedCalled = true
			assert.Equal(t, domain.ProvisioningStatusAttachingCertificate, expectedStatus)
			assert.Contains(t, provisioningError, "timed out")
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, markFailedCalled)
}

func TestAttachingCertificate_AccessDenied_MarksFailed(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:    certARN,
				Status: "ISSUED",
			}, nil
		},
		AttachCertificateToListenerFn: func(ctx context.Context, listenerARN string, certificateARN string) error {
			return errors.New("AccessDeniedException: IAM user is not authorized")
		},
	}

	markFailedCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                            jobID,
				Hostname:                      "app.example.com",
				ProvisioningStatus:            domain.ProvisioningStatusAttachingCertificate,
				CertificateARN:                &certARN,
				CertificateManagedByEliteGate: true,
				LeaseToken:                    &leaseToken,
			}, nil
		},
		MarkProvisioningFailedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, provisioningError string) error {
			markFailedCalled = true
			assert.Contains(t, provisioningError, "access denied")
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, markFailedCalled)
}

func TestAttachingCertificate_TransientError_SchedulesRetry(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:    certARN,
				Status: "ISSUED",
			}, nil
		},
		AttachCertificateToListenerFn: func(ctx context.Context, listenerARN string, certificateARN string) error {
			return errors.New("ThrottlingException: Rate exceeded")
		},
	}

	retryScheduled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                            jobID,
				Hostname:                      "app.example.com",
				ProvisioningStatus:            domain.ProvisioningStatusAttachingCertificate,
				CertificateARN:                &certARN,
				CertificateManagedByEliteGate: true,
				LeaseToken:                    &leaseToken,
			}, nil
		},
		ScheduleProvisioningRetryFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, nextRetryAt time.Time, provisioningError string) error {
			retryScheduled = true
			assert.Contains(t, provisioningError, "transient error")
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, retryScheduled)
}

func TestAttachingCertificate_UnmanagedCert_MarksFailed(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{}
	markFailedCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                            jobID,
				Hostname:                      "app.example.com",
				ProvisioningStatus:            domain.ProvisioningStatusAttachingCertificate,
				CertificateARN:                &certARN,
				CertificateManagedByEliteGate: false,
				LeaseToken:                    &leaseToken,
			}, nil
		},
		MarkProvisioningFailedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, provisioningError string) error {
			markFailedCalled = true
			assert.Contains(t, provisioningError, "not managed by EliteGate")
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, markFailedCalled)
}

func TestDeprovisioning_ManagedCert_Success(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-cert"

	mockAWS := &aws.MockAWSClient{
		DetachCertificateFromListenerFn: func(ctx context.Context, listenerARN, certificateARN string) error {
			assert.Equal(t, certARN, certificateARN)
			return nil
		},
		DeleteCertificateFn: func(ctx context.Context, certificateARN string) error {
			assert.Equal(t, certARN, certificateARN)
			return nil
		},
	}
	markDeprovisionedCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                            jobID,
				Hostname:                      "app.example.com",
				ProvisioningStatus:            domain.ProvisioningStatusDeprovisioning,
				CertificateARN:                &certARN,
				CertificateManagedByEliteGate: true,
				LeaseToken:                    &leaseToken,
			}, nil
		},
		MarkDeprovisionedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID) error {
			markDeprovisionedCalled = true
			assert.Equal(t, jobID, id)
			assert.Equal(t, leaseToken, lToken)
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, markDeprovisionedCalled)
}

func TestDeprovisioning_UnmanagedCert_SkipsAWS(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/custom-cert"

	awsCalled := false
	mockAWS := &aws.MockAWSClient{
		DetachCertificateFromListenerFn: func(ctx context.Context, listenerARN, certificateARN string) error {
			awsCalled = true
			return nil
		},
		DeleteCertificateFn: func(ctx context.Context, certificateARN string) error {
			awsCalled = true
			return nil
		},
	}
	markDeprovisionedCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                            jobID,
				Hostname:                      "app.example.com",
				ProvisioningStatus:            domain.ProvisioningStatusDeprovisioning,
				CertificateARN:                &certARN,
				CertificateManagedByEliteGate: false,
				LeaseToken:                    &leaseToken,
			}, nil
		},
		MarkDeprovisionedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID) error {
			markDeprovisionedCalled = true
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.False(t, awsCalled)
	assert.True(t, markDeprovisionedCalled)
}

func stringPtr(s string) *string {
	return &s
}

func TestDeprovisioning_CrashAfterACMDelete_RecoveryCompletes(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	certARN := "arn:aws:acm:ap-south-1:123456789012:certificate/already-deleted-cert"

	// Simulate AWS returning ResourceNotFoundException on restart
	mockAWS := &aws.MockAWSClient{
		DetachCertificateFromListenerFn: func(ctx context.Context, listenerARN, certificateARN string) error {
			return &acmTypes.ResourceNotFoundException{Message: stringPtr("Listener certificate not found")}
		},
		DeleteCertificateFn: func(ctx context.Context, certificateARN string) error {
			return &acmTypes.ResourceNotFoundException{Message: stringPtr("Certificate not found")}
		},
	}

	markDeprovisionedCalled := false
	mockRepo := &mockProvisioningRepo{
		ClaimNextProvisioningJobFn: func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
			return &domain.ProvisioningJob{
				ID:                            jobID,
				Hostname:                      "app.example.com",
				ProvisioningStatus:            domain.ProvisioningStatusDeprovisioning,
				CertificateARN:                &certARN,
				CertificateManagedByEliteGate: true,
				LeaseToken:                    &leaseToken,
			}, nil
		},
		MarkDeprovisionedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID) error {
			markDeprovisionedCalled = true
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, markDeprovisionedCalled)
}

func TestProvisioningWorkflow_EndToEnd_ManagedFlagPersistence(t *testing.T) {
	jobID := uuid.New()
	leaseToken := uuid.New()
	mockCertARN := "arn:aws:acm:ap-south-1:123456789012:certificate/test-managed-cert"

	var managedFlagInState *bool

	mockAWS := &aws.MockAWSClient{
		RequestCertificateFn: func(ctx context.Context, hostname string, idempotencyToken string) (string, error) {
			return mockCertARN, nil
		},
		DescribeCertificateFn: func(ctx context.Context, certificateARN string) (*aws.CertificateDetails, error) {
			return &aws.CertificateDetails{
				ARN:             certificateARN,
				Status:          "ISSUED",
				ValidationName:  "_a.app.example.com",
				ValidationValue: "_b.acm-validations.aws",
			}, nil
		},
		AttachCertificateToListenerFn: func(ctx context.Context, listenerARN, certificateARN string) error {
			return nil
		},
	}

	completedCalled := false
	failedCalled := false

	mockRepo := &mockProvisioningRepo{
		AdvanceProvisioningStateFn: func(ctx context.Context, params storage.AdvanceProvisioningParams) error {
			if params.CertificateManagedByEliteGate != nil {
				managedFlagInState = params.CertificateManagedByEliteGate
			}
			return nil
		},
		MarkProvisioningCompletedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID) error {
			completedCalled = true
			return nil
		},
		MarkProvisioningFailedFn: func(ctx context.Context, id uuid.UUID, lToken uuid.UUID, expectedStatus string, provisioningError string) error {
			failedCalled = true
			t.Fatalf("Provisioning unexpectedly failed: %s", provisioningError)
			return nil
		},
	}

	provisioner := newTestProvisioner(mockRepo, mockAWS)

	// Step 1: requesting_certificate state -> requests ACM cert, persists managed=true
	job1 := &domain.ProvisioningJob{
		ID:                 jobID,
		Hostname:           "app.example.com",
		ProvisioningStatus: domain.ProvisioningStatusRequestingCertificate,
		LeaseToken:         &leaseToken,
	}
	mockRepo.ClaimNextProvisioningJobFn = func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
		return job1, nil
	}
	processed, err := provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	require.NotNil(t, managedFlagInState, "AdvanceProvisioningState must have received CertificateManagedByEliteGate")
	assert.True(t, *managedFlagInState, "CertificateManagedByEliteGate must be set to true upon certificate request")

	// Step 2: attaching_certificate state -> check job has CertificateManagedByEliteGate = true
	job2 := &domain.ProvisioningJob{
		ID:                            jobID,
		Hostname:                      "app.example.com",
		ProvisioningStatus:            domain.ProvisioningStatusAttachingCertificate,
		CertificateARN:                &mockCertARN,
		CertificateManagedByEliteGate: true, // Persisted value loaded from DB
		LeaseToken:                    &leaseToken,
	}
	mockRepo.ClaimNextProvisioningJobFn = func(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error) {
		return job2, nil
	}
	processed, err = provisioner.ProcessNextJob(context.Background())
	require.NoError(t, err)
	assert.True(t, processed)
	assert.False(t, failedCalled, "Worker must NOT fail with 'certificate is not managed by EliteGate'")
	assert.True(t, completedCalled, "Provisioning must complete successfully")
}
