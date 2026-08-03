package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"elitegate/internal/aws"
	"elitegate/internal/domain"
	"elitegate/internal/metrics"
	"elitegate/internal/storage"

	acmTypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// GenerateACMIdempotencyToken creates a deterministic, valid ACM idempotency token from a custom domain UUID.
func GenerateACMIdempotencyToken(domainID uuid.UUID) string {
	h := sha256.Sum256([]byte(domainID.String()))
	hexStr := hex.EncodeToString(h[:])
	return "eg" + hexStr[:30]
}

// CalculateRetryDelay returns exponential backoff capped at 5m: 15s -> 30s -> 1m -> 2m -> 5m max.
func CalculateRetryDelay(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return 15 * time.Second
	case attempts == 2:
		return 30 * time.Second
	case attempts == 3:
		return 1 * time.Minute
	case attempts == 4:
		return 2 * time.Minute
	default:
		return 5 * time.Minute
	}
}

// ErrorCategory classifies AWS errors into transient vs terminal failures.
type ErrorCategory int

const (
	ErrorCategoryTransient ErrorCategory = iota
	ErrorCategoryTerminal
)

func (c ErrorCategory) String() string {
	if c == ErrorCategoryTerminal {
		return metrics.CategoryTerminal
	}
	return metrics.CategoryTransient
}

// ClassifyAWSError determines if an error from AWS ACM operations is transient or terminal.
func ClassifyAWSError(err error) (ErrorCategory, string) {
	if err == nil {
		return ErrorCategoryTransient, ""
	}

	errMsg := err.Error()
	lowered := strings.ToLower(errMsg)

	var notFoundErr *acmTypes.ResourceNotFoundException
	var invalidParamErr *acmTypes.InvalidParameterException
	var accessDeniedErr *acmTypes.AccessDeniedException

	if errors.As(err, &notFoundErr) || strings.Contains(lowered, "resourcenotfoundexception") {
		return ErrorCategoryTerminal, "ACM certificate resource not found"
	}
	if errors.As(err, &invalidParamErr) || strings.Contains(lowered, "invalidparameterexception") || strings.Contains(lowered, "invalid domain") {
		return ErrorCategoryTerminal, "Invalid parameters for ACM certificate request"
	}
	if errors.As(err, &accessDeniedErr) || strings.Contains(lowered, "accessdeniedexception") || strings.Contains(lowered, "access denied") {
		return ErrorCategoryTerminal, "AWS IAM access denied for ACM operation"
	}

	var throttlingErr *acmTypes.ThrottlingException
	var limitExceededErr *acmTypes.LimitExceededException
	if errors.As(err, &throttlingErr) || errors.As(err, &limitExceededErr) ||
		strings.Contains(lowered, "throttling") || strings.Contains(lowered, "toomanyrequests") ||
		strings.Contains(lowered, "limitexceeded") || strings.Contains(lowered, "timeout") ||
		strings.Contains(lowered, "connection refused") || strings.Contains(lowered, "503") ||
		strings.Contains(lowered, "500") {
		return ErrorCategoryTransient, "AWS ACM service transient error / throttling"
	}

	return ErrorCategoryTransient, "AWS ACM operation failed transiently: " + errMsg
}

func isResourceNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	var notFoundErr *acmTypes.ResourceNotFoundException
	if errors.As(err, &notFoundErr) {
		return true
	}
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "resourcenotfoundexception") ||
		strings.Contains(lowered, "not found") ||
		strings.Contains(lowered, "does not exist")
}

// ProvisioningRepository declares the database operations required by the provisioner.
type ProvisioningRepository interface {
	ClaimNextProvisioningJob(ctx context.Context, workerID string, lockTimeout time.Duration) (*domain.ProvisioningJob, error)
	AdvanceProvisioningState(ctx context.Context, params storage.AdvanceProvisioningParams) error
	ScheduleProvisioningPoll(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, expectedStatus string, nextPollAt time.Time) error
	ScheduleProvisioningRetry(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, expectedStatus string, nextRetryAt time.Time, provisioningError string) error
	MarkProvisioningFailed(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, expectedStatus string, provisioningError string) error
	MarkProvisioningCompleted(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID) error
	MarkDeprovisioned(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID) error
	MarkDeprovisionFailed(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID, errStr string) error
	ReleaseProvisioningLease(ctx context.Context, id uuid.UUID, leaseToken uuid.UUID) error
}

// Provisioner coordinates AWS ACM certificate requests, validation, and listener attachment.
type Provisioner struct {
	repo         ProvisioningRepository
	certificates aws.CertificateManager
	loadBalancer aws.LoadBalancerManager
	workerID     string
	listenerARN  string
	metrics      *metrics.CustomDomainMetrics
	logger       zerolog.Logger
	now          func() time.Time
	pollInterval time.Duration
}

// NewProvisioner creates a new ACM Provisioner.
func NewProvisioner(
	repo ProvisioningRepository,
	certificates aws.CertificateManager,
	loadBalancer aws.LoadBalancerManager,
	workerID string,
	listenerARN string,
	logger zerolog.Logger,
) *Provisioner {
	if workerID == "" {
		workerID = "worker-" + uuid.New().String()[:8]
	}
	return &Provisioner{
		repo:         repo,
		certificates: certificates,
		loadBalancer: loadBalancer,
		workerID:     workerID,
		listenerARN:  listenerARN,
		logger:       logger,
		now:          time.Now,
		pollInterval: 30 * time.Second,
	}
}

func (p *Provisioner) WithMetrics(m *metrics.CustomDomainMetrics) *Provisioner {
	p.metrics = m
	return p
}

// ProcessNextJob claims and processes one eligible custom domain provisioning job.
func (p *Provisioner) ProcessNextJob(ctx context.Context) (bool, error) {
	job, err := p.repo.ClaimNextProvisioningJob(ctx, p.workerID, 5*time.Minute)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to claim next provisioning job")
		return false, err
	}
	if job == nil {
		return false, nil
	}

	op := metrics.OpProvision
	if job.ProvisioningStatus == domain.ProvisioningStatusDeprovisioning {
		op = metrics.OpDeprovision
	}

	if p.metrics != nil {
		p.metrics.JobsClaimed.WithLabelValues(op).Inc()
		p.metrics.JobsActive.WithLabelValues(op).Inc()
		defer p.metrics.JobsActive.WithLabelValues(op).Dec()
	}

	p.logger.Info().
		Str("job_id", job.ID.String()).
		Str("project_id", job.ProjectID.String()).
		Str("hostname", job.Hostname).
		Str("provisioning_status", job.ProvisioningStatus).
		Int("attempts", job.ProvisioningAttempts).
		Msg("provisioning job claimed")

	switch job.ProvisioningStatus {
	case domain.ProvisioningStatusRequestingCertificate:
		err = p.handleRequestingCertificate(ctx, job)
	case domain.ProvisioningStatusWaitingForValidationRecord:
		err = p.handleWaitingForValidationRecord(ctx, job)
	case domain.ProvisioningStatusWaitingForDNS:
		err = p.handleWaitingForDNS(ctx, job)
	case domain.ProvisioningStatusAttachingCertificate:
		err = p.handleAttachingCertificate(ctx, job)
	case domain.ProvisioningStatusDeprovisioning:
		err = p.handleDeprovisioning(ctx, job)
	default:
		p.logger.Warn().
			Str("job_id", job.ID.String()).
			Str("status", job.ProvisioningStatus).
			Msg("unhandled provisioning status; releasing lease")
		if job.LeaseToken != nil {
			_ = p.repo.ReleaseProvisioningLease(ctx, job.ID, *job.LeaseToken)
		}
		return true, nil
	}

	if err != nil {
		if p.metrics != nil {
			p.metrics.WorkerLastFailureTimestamp.Set(float64(p.now().Unix()))
		}
		p.logger.Error().
			Err(err).
			Str("job_id", job.ID.String()).
			Str("status", job.ProvisioningStatus).
			Msg("failed to process provisioning job transition")
		return true, err
	}

	if p.metrics != nil {
		p.metrics.WorkerLastSuccessTimestamp.Set(float64(p.now().Unix()))
		if job.ProvisioningStatus == domain.ProvisioningStatusAttachingCertificate && job.ProvisioningStartedAt != nil {
			duration := p.now().Sub(*job.ProvisioningStartedAt).Seconds()
			p.metrics.ProvisionDurationSeconds.WithLabelValues("success").Observe(duration)
			p.metrics.JobsCompleted.WithLabelValues(metrics.OpProvision).Inc()
		} else if job.ProvisioningStatus == domain.ProvisioningStatusDeprovisioning && job.ProvisioningStartedAt != nil {
			duration := p.now().Sub(*job.ProvisioningStartedAt).Seconds()
			p.metrics.DeprovisionDurationSeconds.WithLabelValues("success").Observe(duration)
			p.metrics.JobsCompleted.WithLabelValues(metrics.OpDeprovision).Inc()
		}
	}

	return true, nil
}

func (p *Provisioner) handleRequestingCertificate(ctx context.Context, job *domain.ProvisioningJob) error {
	if job.LeaseToken == nil {
		return errors.New("job lease token missing")
	}
	leaseToken := *job.LeaseToken

	managed := true
	if job.CertificateARN != nil && *job.CertificateARN != "" {
		p.logger.Info().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Str("certificate_arn", *job.CertificateARN).
			Msg("certificate ARN already exists; advancing state to waiting_for_validation_record")

		pendingValidation := "pending_validation"
		now := p.now()
		return p.repo.AdvanceProvisioningState(ctx, storage.AdvanceProvisioningParams{
			ID:                            job.ID,
			LeaseToken:                    leaseToken,
			ExpectedStatus:                domain.ProvisioningStatusRequestingCertificate,
			NewStatus:                     domain.ProvisioningStatusWaitingForValidationRecord,
			CertificateARN:                job.CertificateARN,
			CertificateStatus:             &pendingValidation,
			CertificateManagedByEliteGate: &managed,
			CertificateRequestedAt:        &now,
			NextRetryAt:                   &now,
		})
	}

	idempotencyToken := GenerateACMIdempotencyToken(job.ID)

	p.logger.Info().
		Str("job_id", job.ID.String()).
		Str("hostname", job.Hostname).
		Msg("requesting ACM certificate")

	certARN, err := p.certificates.RequestCertificate(ctx, job.Hostname, idempotencyToken)
	if err != nil {
		cat, reason := ClassifyAWSError(err)
		p.logger.Error().
			Err(err).
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("failed to request ACM certificate")

		if cat == ErrorCategoryTerminal {
			return p.repo.MarkProvisioningFailed(
				ctx,
				job.ID,
				leaseToken,
				domain.ProvisioningStatusRequestingCertificate,
				reason,
			)
		}

		retryAt := p.now().Add(CalculateRetryDelay(job.ProvisioningAttempts + 1))
		return p.repo.ScheduleProvisioningRetry(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusRequestingCertificate,
			retryAt,
			reason,
		)
	}

	p.logger.Info().
		Str("job_id", job.ID.String()).
		Str("hostname", job.Hostname).
		Str("certificate_arn", certARN).
		Msg("ACM certificate requested successfully; advancing state to waiting_for_validation_record")

	pendingValidation := "pending_validation"
	now := p.now()
	return p.repo.AdvanceProvisioningState(ctx, storage.AdvanceProvisioningParams{
		ID:                            job.ID,
		LeaseToken:                    leaseToken,
		ExpectedStatus:                domain.ProvisioningStatusRequestingCertificate,
		NewStatus:                     domain.ProvisioningStatusWaitingForValidationRecord,
		CertificateARN:                &certARN,
		CertificateStatus:             &pendingValidation,
		CertificateManagedByEliteGate: &managed,
		CertificateRequestedAt:        &now,
		NextRetryAt:                   &now,
	})
}

func (p *Provisioner) handleWaitingForValidationRecord(ctx context.Context, job *domain.ProvisioningJob) error {
	if job.LeaseToken == nil {
		return errors.New("job lease token missing")
	}
	leaseToken := *job.LeaseToken

	if job.CertificateARN == nil || *job.CertificateARN == "" {
		p.logger.Error().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("missing certificate ARN in waiting_for_validation_record state")

		return p.repo.MarkProvisioningFailed(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusWaitingForValidationRecord,
			"Missing ACM certificate ARN in waiting_for_validation_record state",
		)
	}

	p.logger.Info().
		Str("job_id", job.ID.String()).
		Str("hostname", job.Hostname).
		Str("certificate_arn", *job.CertificateARN).
		Msg("describing ACM certificate for validation records")

	details, err := p.certificates.DescribeCertificate(ctx, *job.CertificateARN)
	if err != nil {
		cat, reason := ClassifyAWSError(err)
		p.logger.Error().
			Err(err).
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("failed to describe ACM certificate")

		if cat == ErrorCategoryTerminal {
			return p.repo.MarkProvisioningFailed(
				ctx,
				job.ID,
				leaseToken,
				domain.ProvisioningStatusWaitingForValidationRecord,
				reason,
			)
		}

		retryAt := p.now().Add(CalculateRetryDelay(job.ProvisioningAttempts + 1))
		return p.repo.ScheduleProvisioningRetry(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusWaitingForValidationRecord,
			retryAt,
			reason,
		)
	}

	if details.Status == "FAILED" || details.FailureReason != "" {
		failMsg := fmt.Sprintf("ACM certificate failed validation: %s", details.FailureReason)
		p.logger.Error().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Str("acm_status", details.Status).
			Str("failure_reason", details.FailureReason).
			Msg("ACM certificate in terminal failure state")

		return p.repo.MarkProvisioningFailed(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusWaitingForValidationRecord,
			failMsg,
		)
	}

	if details.ValidationName == "" || details.ValidationValue == "" {
		p.logger.Info().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("ACM validation CNAME not available yet; scheduling normal poll")

		nextPollAt := p.now().Add(p.pollInterval)
		return p.repo.ScheduleProvisioningPoll(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusWaitingForValidationRecord,
			nextPollAt,
		)
	}

	p.logger.Info().
		Str("job_id", job.ID.String()).
		Str("hostname", job.Hostname).
		Str("validation_name", details.ValidationName).
		Msg("ACM validation record available; advancing state to waiting_for_dns")

	nextPollAt := p.now().Add(p.pollInterval)
	certStatus := details.Status
	return p.repo.AdvanceProvisioningState(ctx, storage.AdvanceProvisioningParams{
		ID:                         job.ID,
		LeaseToken:                 leaseToken,
		ExpectedStatus:             domain.ProvisioningStatusWaitingForValidationRecord,
		NewStatus:                  domain.ProvisioningStatusWaitingForDNS,
		CertificateValidationName:  &details.ValidationName,
		CertificateValidationValue: &details.ValidationValue,
		CertificateStatus:          &certStatus,
		NextRetryAt:                &nextPollAt,
	})
}

// SanitizeACMFailureReason maps AWS ACM failure reasons into sanitized internal messages.
func SanitizeACMFailureReason(reason string) string {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "DOMAIN_VALIDATION_TIMED_OUT", "VALIDATION_TIMED_OUT":
		return "ACM domain validation timed out"
	case "FAILURE_REASON_NO_AVAILABLE_CONTACTS":
		return "ACM validation failed: no available contacts"
	case "FAILURE_REASON_ADDITIONAL_VERIFICATION_REQUIRED":
		return "ACM validation failed: additional verification required"
	case "REVOKED":
		return "ACM certificate was revoked"
	case "EXPIRED":
		return "ACM certificate expired"
	case "INACTIVE":
		return "ACM certificate is inactive"
	default:
		if reason != "" {
			return "ACM certificate failed: " + reason
		}
		return "ACM certificate issuance failed"
	}
}

func (p *Provisioner) handleWaitingForDNS(ctx context.Context, job *domain.ProvisioningJob) error {
	if job.LeaseToken == nil {
		return errors.New("job lease token missing")
	}
	leaseToken := *job.LeaseToken

	if job.CertificateARN == nil || *job.CertificateARN == "" {
		p.logger.Error().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("missing certificate ARN in waiting_for_dns state")

		return p.repo.MarkProvisioningFailed(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusWaitingForDNS,
			"Missing ACM certificate ARN in waiting_for_dns state",
		)
	}

	p.logger.Info().
		Str("job_id", job.ID.String()).
		Str("hostname", job.Hostname).
		Msg("polling ACM certificate status for issuance")

	details, err := p.certificates.DescribeCertificate(ctx, *job.CertificateARN)
	if err != nil {
		cat, reason := ClassifyAWSError(err)
		p.logger.Error().
			Err(err).
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("failed to describe ACM certificate")

		if cat == ErrorCategoryTerminal {
			return p.repo.MarkProvisioningFailed(
				ctx,
				job.ID,
				leaseToken,
				domain.ProvisioningStatusWaitingForDNS,
				reason,
			)
		}

		retryAt := p.now().Add(CalculateRetryDelay(job.ProvisioningAttempts + 1))
		return p.repo.ScheduleProvisioningRetry(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusWaitingForDNS,
			retryAt,
			reason,
		)
	}

	statusUpper := strings.ToUpper(strings.TrimSpace(details.Status))

	switch statusUpper {
	case "ISSUED":
		p.logger.Info().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("ACM certificate issued successfully; advancing state to attaching_certificate")

		certStatus := "issued"
		issuedAt := details.IssuedAt
		if issuedAt == nil {
			now := p.now()
			issuedAt = &now
		}

		now := p.now()
		return p.repo.AdvanceProvisioningState(ctx, storage.AdvanceProvisioningParams{
			ID:                  job.ID,
			LeaseToken:          leaseToken,
			ExpectedStatus:      domain.ProvisioningStatusWaitingForDNS,
			NewStatus:           domain.ProvisioningStatusAttachingCertificate,
			CertificateStatus:   &certStatus,
			CertificateIssuedAt: issuedAt,
			NextRetryAt:         &now,
		})

	case "PENDING_VALIDATION":
		p.logger.Info().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("ACM certificate pending DNS validation; scheduling next poll")

		nextPollAt := p.now().Add(p.pollInterval)
		return p.repo.ScheduleProvisioningPoll(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusWaitingForDNS,
			nextPollAt,
		)

	case "FAILED", "VALIDATION_TIMED_OUT", "REVOKED", "EXPIRED", "INACTIVE":
		failMsg := SanitizeACMFailureReason(details.FailureReason)
		if details.FailureReason == "" {
			failMsg = SanitizeACMFailureReason(statusUpper)
		}

		p.logger.Error().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Str("acm_status", statusUpper).
			Msg("ACM certificate in terminal failed status")

		return p.repo.MarkProvisioningFailed(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusWaitingForDNS,
			failMsg,
		)

	default:
		if job.ProvisioningAttempts >= 5 {
			p.logger.Error().
				Str("job_id", job.ID.String()).
				Str("hostname", job.Hostname).
				Str("acm_status", statusUpper).
				Msg("ACM certificate status unknown after max retries; marking failed")

			return p.repo.MarkProvisioningFailed(
				ctx,
				job.ID,
				leaseToken,
				domain.ProvisioningStatusWaitingForDNS,
				"Unknown ACM certificate status: "+statusUpper,
			)
		}

		p.logger.Warn().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Str("acm_status", statusUpper).
			Msg("unrecognized ACM status; scheduling transient retry")

		retryAt := p.now().Add(CalculateRetryDelay(job.ProvisioningAttempts + 1))
		return p.repo.ScheduleProvisioningRetry(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusWaitingForDNS,
			retryAt,
			"Unrecognized ACM status: "+statusUpper,
		)
	}
}

func (p *Provisioner) handleAttachingCertificate(ctx context.Context, job *domain.ProvisioningJob) error {
	if job.LeaseToken == nil {
		return errors.New("job lease token missing")
	}
	leaseToken := *job.LeaseToken

	if job.CertificateARN == nil || *job.CertificateARN == "" {
		p.logger.Error().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("missing certificate ARN in attaching_certificate state")

		return p.repo.MarkProvisioningFailed(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusAttachingCertificate,
			"Missing ACM certificate ARN in attaching_certificate state",
		)
	}

	if !job.CertificateManagedByEliteGate {
		p.logger.Error().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("certificate is not managed by EliteGate")

		return p.repo.MarkProvisioningFailed(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusAttachingCertificate,
			"Certificate not managed by EliteGate",
		)
	}

	// 1. Re-verify ACM certificate status via DescribeCertificate
	details, err := p.certificates.DescribeCertificate(ctx, *job.CertificateARN)
	if err != nil {
		cat, reason := ClassifyAWSError(err)
		p.logger.Error().
			Err(err).
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("failed to re-verify ACM certificate status")

		if cat == ErrorCategoryTerminal {
			return p.repo.MarkProvisioningFailed(
				ctx,
				job.ID,
				leaseToken,
				domain.ProvisioningStatusAttachingCertificate,
				reason,
			)
		}

		retryAt := p.now().Add(CalculateRetryDelay(job.ProvisioningAttempts + 1))
		return p.repo.ScheduleProvisioningRetry(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusAttachingCertificate,
			retryAt,
			reason,
		)
	}

	statusUpper := strings.ToUpper(strings.TrimSpace(details.Status))

	switch statusUpper {
	case "ISSUED":
		// Continue to ALB attachment
	case "PENDING_VALIDATION":
		p.logger.Info().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("ACM certificate unexpectedly pending validation; moving back to waiting_for_dns polling")

		nextPollAt := p.now().Add(p.pollInterval)
		return p.repo.ScheduleProvisioningPoll(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusAttachingCertificate,
			nextPollAt,
		)
	case "FAILED", "VALIDATION_TIMED_OUT", "REVOKED", "EXPIRED", "INACTIVE":
		failMsg := SanitizeACMFailureReason(details.FailureReason)
		if details.FailureReason == "" {
			failMsg = SanitizeACMFailureReason(statusUpper)
		}

		p.logger.Error().
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Str("acm_status", statusUpper).
			Msg("ACM certificate in terminal failed status during attachment re-verification")

		return p.repo.MarkProvisioningFailed(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusAttachingCertificate,
			failMsg,
		)
	default:
		retryAt := p.now().Add(CalculateRetryDelay(job.ProvisioningAttempts + 1))
		return p.repo.ScheduleProvisioningRetry(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusAttachingCertificate,
			retryAt,
			"Unrecognized ACM status during attachment: "+statusUpper,
		)
	}

	// 2. Attach Certificate to ALB HTTPS Listener
	p.logger.Info().
		Str("job_id", job.ID.String()).
		Str("hostname", job.Hostname).
		Msg("attaching ACM certificate to ALB HTTPS listener")

	err = p.loadBalancer.AttachCertificateToListener(ctx, p.listenerARN, *job.CertificateARN)
	if err != nil {
		cat, reason := ClassifyAWSError(err)
		p.logger.Error().
			Err(err).
			Str("job_id", job.ID.String()).
			Str("hostname", job.Hostname).
			Msg("failed to attach certificate to ALB listener")

		if cat == ErrorCategoryTerminal {
			return p.repo.MarkProvisioningFailed(
				ctx,
				job.ID,
				leaseToken,
				domain.ProvisioningStatusAttachingCertificate,
				reason,
			)
		}

		retryAt := p.now().Add(CalculateRetryDelay(job.ProvisioningAttempts + 1))
		return p.repo.ScheduleProvisioningRetry(
			ctx,
			job.ID,
			leaseToken,
			domain.ProvisioningStatusAttachingCertificate,
			retryAt,
			reason,
		)
	}

	// 3. Complete Provisioning & Mark Active
	p.logger.Info().
		Str("job_id", job.ID.String()).
		Str("hostname", job.Hostname).
		Msg("ALB attachment succeeded; marking domain active and completing provisioning")

	return p.repo.MarkProvisioningCompleted(ctx, job.ID, leaseToken)
}

func (p *Provisioner) handleDeprovisioning(ctx context.Context, job *domain.ProvisioningJob) error {
	if job.LeaseToken == nil {
		return errors.New("job lease token missing")
	}
	leaseToken := *job.LeaseToken

	// Check certificate ownership & ARN
	if job.CertificateARN != nil && *job.CertificateARN != "" && job.CertificateManagedByEliteGate {
		certARN := *job.CertificateARN

		// Step 1: Detach certificate from ALB HTTPS listener
		if p.listenerARN != "" && p.loadBalancer != nil {
			p.logger.Info().
				Str("job_id", job.ID.String()).
				Str("hostname", job.Hostname).
				Str("certificate_arn", certARN).
				Str("listener_arn", p.listenerARN).
				Msg("detaching certificate from ALB listener")

			err := p.loadBalancer.DetachCertificateFromListener(ctx, p.listenerARN, certARN)
			if err != nil && !isResourceNotFoundErr(err) {
				cat, reason := ClassifyAWSError(err)
				p.logger.Error().
					Err(err).
					Int("category", int(cat)).
					Str("reason", reason).
					Msg("failed to detach certificate from ALB listener")

				if cat == ErrorCategoryTransient {
					delay := CalculateRetryDelay(job.ProvisioningAttempts + 1)
					nextRetry := p.now().Add(delay)
					return p.repo.ScheduleProvisioningRetry(ctx, job.ID, leaseToken, domain.ProvisioningStatusDeprovisioning, nextRetry, reason)
				}
				// Explicit Guard: Stop immediately if detach fails terminally (e.g. default listener certificate)
				return p.repo.MarkDeprovisionFailed(ctx, job.ID, leaseToken, reason)
			}
		}

		// Step 2: Delete ACM Certificate (called ONLY after detach returns success or already unattached)
		if p.certificates != nil {
			p.logger.Info().
				Str("job_id", job.ID.String()).
				Str("hostname", job.Hostname).
				Str("certificate_arn", certARN).
				Msg("deleting ACM certificate")

			err := p.certificates.DeleteCertificate(ctx, certARN)
			if err != nil && !isResourceNotFoundErr(err) {
				cat, reason := ClassifyAWSError(err)
				p.logger.Error().
					Err(err).
					Int("category", int(cat)).
					Str("reason", reason).
					Msg("failed to delete ACM certificate")

				if cat == ErrorCategoryTransient {
					delay := CalculateRetryDelay(job.ProvisioningAttempts + 1)
					nextRetry := p.now().Add(delay)
					return p.repo.ScheduleProvisioningRetry(ctx, job.ID, leaseToken, domain.ProvisioningStatusDeprovisioning, nextRetry, reason)
				}
				return p.repo.MarkDeprovisionFailed(ctx, job.ID, leaseToken, reason)
			}
		}
	}

	// Step 3: Finalize DB state & soft-delete
	p.logger.Info().
		Str("job_id", job.ID.String()).
		Str("hostname", job.Hostname).
		Msg("marking domain deprovisioned and soft-deleting")

	return p.repo.MarkDeprovisioned(ctx, job.ID, leaseToken)
}
