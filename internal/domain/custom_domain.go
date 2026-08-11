package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	CustomDomainStatusPendingVerification = "pending_verification"
	CustomDomainStatusVerified            = "verified"
	CustomDomainStatusPendingDNS          = "pending_dns"
	CustomDomainStatusPendingCertificate  = "pending_certificate"
	CustomDomainStatusActive              = "active"
	CustomDomainStatusVerificationFailed  = "verification_failed"
	CustomDomainStatusCertificateFailed   = "certificate_failed"
	CustomDomainStatusDisabled            = "disabled"
	CustomDomainStatusDeleting            = "deleting"
)

const (
	CustomDomainRoutingStatusPending = "pending"
	CustomDomainRoutingStatusReady   = "ready"
	CustomDomainRoutingStatusFailed  = "failed"
)

const (
	ProvisioningStatusNotStarted                 = "not_started"
	ProvisioningStatusRequestingCertificate      = "requesting_certificate"
	ProvisioningStatusWaitingForValidationRecord = "waiting_for_validation_record"
	ProvisioningStatusWaitingForDNS              = "waiting_for_dns"
	ProvisioningStatusWaitingForCertificate      = "waiting_for_certificate"
	ProvisioningStatusAttachingCertificate       = "attaching_certificate"
	ProvisioningStatusCompleted                  = "completed"
	ProvisioningStatusFailed                     = "failed"
	ProvisioningStatusDeprovisioning             = "deprovisioning"
	ProvisioningStatusDeprovisioned              = "deprovisioned"
	ProvisioningStatusDeprovisionFailed          = "deprovision_failed"
)

const (
	CertificateStatusPendingValidation = "pending_validation"
	CertificateStatusIssued            = "issued"
	CertificateStatusFailed            = "failed"
)

type ActivationState string

const (
	ActivationQueued        ActivationState = "queued"
	ActivationInProgress    ActivationState = "in_progress"
	ActivationAlreadyActive ActivationState = "already_active"
)

// ProvisioningStatusResponse represents public, sanitized provisioning status details.
type ProvisioningStatusResponse struct {
	ID                         uuid.UUID  `json:"id"`
	Hostname                   string     `json:"hostname"`
	Status                     string     `json:"status"`
	RoutingStatus              string     `json:"routingStatus"`
	ProvisioningStatus         string     `json:"provisioningStatus"`
	CertificateStatus          *string    `json:"certificateStatus,omitempty"`
	CertificateValidationName  *string    `json:"certificateValidationName,omitempty"`
	CertificateValidationValue *string    `json:"certificateValidationValue,omitempty"`
	LastError                  *string    `json:"lastError,omitempty"`
	Attempts                   int        `json:"attempts"`
	NextRetryAt                *time.Time `json:"nextRetryAt,omitempty"`
	CertificateIssuedAt        *time.Time `json:"certificateIssuedAt,omitempty"`
	CertificateAttachedAt      *time.Time `json:"certificateAttachedAt,omitempty"`
	ActivatedAt                *time.Time `json:"activatedAt,omitempty"`
}

// CustomDomain represents a custom domain entity in the system.
type CustomDomain struct {
	ID                            uuid.UUID  `json:"id"`
	ProjectID                     uuid.UUID  `json:"project_id"`
	Hostname                      string     `json:"hostname"`
	Status                        string     `json:"status"`
	VerificationTokenHash         string     `json:"-"`
	VerificationRecordName        string     `json:"verification_record_name,omitempty"`
	CertificateARN                *string    `json:"certificate_arn,omitempty"`
	CertificateStatus             *string    `json:"certificate_status,omitempty"`
	FailureReason                 *string    `json:"failure_reason,omitempty"`
	VerifiedAt                    *time.Time `json:"verified_at,omitempty"`
	ActivatedAt                   *time.Time `json:"activated_at,omitempty"`
	LastCheckedAt                 *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt                     time.Time  `json:"created_at"`
	UpdatedAt                     time.Time  `json:"updated_at"`
	DeletedAt                     *time.Time `json:"-"`
	RoutingTarget                 *string    `json:"routing_target,omitempty"`
	RoutingStatus                 string     `json:"routing_status,omitempty"`
	RoutingCheckedAt              *time.Time `json:"routing_checked_at,omitempty"`
	RoutingError                  *string    `json:"routing_error,omitempty"`
	CertificateManagedByEliteGate bool       `json:"certificate_managed_by_elitegate"`
	ProvisioningStatus            string     `json:"provisioning_status"`
	CertificateValidationName     *string    `json:"certificate_validation_name,omitempty"`
	CertificateValidationValue    *string    `json:"certificate_validation_value,omitempty"`
	CertificateRequestedAt        *time.Time `json:"certificate_requested_at,omitempty"`
	CertificateIssuedAt           *time.Time `json:"certificate_issued_at,omitempty"`
	CertificateAttachedAt         *time.Time `json:"certificate_attached_at,omitempty"`
	ProvisioningStartedAt         *time.Time `json:"provisioning_started_at,omitempty"`
	ProvisioningCompletedAt       *time.Time `json:"provisioning_completed_at,omitempty"`
	DeprovisioningStartedAt       *time.Time `json:"deprovisioning_started_at,omitempty"`
	ProvisioningError             *string    `json:"provisioning_error,omitempty"`
	ProvisioningAttempts          int        `json:"provisioning_attempts"`
	NextRetryAt                   *time.Time `json:"next_retry_at,omitempty"`
	ListenerRuleARN               *string    `json:"-"`
	ListenerRulePriority          *int       `json:"-"`
	LockedAt                      *time.Time `json:"-"`
	LockedBy                      *string    `json:"-"`
	LeaseToken                    *uuid.UUID `json:"-"`
}

// ProvisioningJob represents an actionable domain provisioning task for workers.
type ProvisioningJob struct {
	ID                            uuid.UUID  `json:"id"`
	ProjectID                     uuid.UUID  `json:"project_id"`
	Hostname                      string     `json:"hostname"`
	Status                        string     `json:"status"`
	RoutingStatus                 string     `json:"routing_status"`
	ProvisioningStatus            string     `json:"provisioning_status"`
	CertificateARN                *string    `json:"certificate_arn,omitempty"`
	CertificateStatus             *string    `json:"certificate_status,omitempty"`
	CertificateManagedByEliteGate bool       `json:"certificate_managed_by_elitegate"`
	CertificateValidationName     *string    `json:"certificate_validation_name,omitempty"`
	CertificateValidationValue    *string    `json:"certificate_validation_value,omitempty"`
	ListenerRuleARN               *string    `json:"-"`
	ListenerRulePriority          *int       `json:"-"`
	ProvisioningAttempts          int        `json:"provisioning_attempts"`
	NextRetryAt                   *time.Time `json:"next_retry_at,omitempty"`
	ProvisioningStartedAt         *time.Time `json:"provisioning_started_at,omitempty"`
	LockedAt                      *time.Time `json:"locked_at,omitempty"`
	LockedBy                      *string    `json:"locked_by,omitempty"`
	LeaseToken                    *uuid.UUID `json:"lease_token,omitempty"`
	DeletedAt                     *time.Time `json:"deleted_at,omitempty"`
}

// ProvisioningStatusDTO represents sanitized provisioning status returned to frontends.
type ProvisioningStatusDTO struct {
	ID                         uuid.UUID  `json:"id"`
	Hostname                   string     `json:"hostname"`
	Status                     string     `json:"status"`
	RoutingStatus              string     `json:"routing_status"`
	ProvisioningStatus         string     `json:"provisioning_status"`
	CertificateValidationName  *string    `json:"certificate_validation_name,omitempty"`
	CertificateValidationValue *string    `json:"certificate_validation_value,omitempty"`
	CertificateStatus          *string    `json:"certificate_status,omitempty"`
	SafeErrorMessage           *string    `json:"error_message,omitempty"`
	ActivatedAt                *time.Time `json:"activated_at,omitempty"`
}

// VerificationRecord holds details for DNS verification of a custom domain.
type VerificationRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CreateCustomDomainRequest is the request body used when registering a domain.
type CreateCustomDomainRequest struct {
	Hostname string `json:"hostname" binding:"required"`
}

// CreateCustomDomainResponse is returned after registering a custom domain.
type CreateCustomDomainResponse struct {
	ID                 uuid.UUID          `json:"id"`
	ProjectID          uuid.UUID          `json:"project_id"`
	Hostname           string             `json:"hostname"`
	Status             string             `json:"status"`
	VerificationRecord VerificationRecord `json:"verification_record"`
	CreatedAt          time.Time          `json:"created_at"`
}
