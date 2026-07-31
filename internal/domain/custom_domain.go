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

// CustomDomain represents a custom domain entity in the system.
type CustomDomain struct {
	ID                     uuid.UUID  `json:"id"`
	ProjectID              uuid.UUID  `json:"project_id"`
	Hostname               string     `json:"hostname"`
	Status                 string     `json:"status"`
	VerificationTokenHash  string     `json:"-"`
	VerificationRecordName string     `json:"verification_record_name,omitempty"`
	CertificateARN         *string    `json:"certificate_arn,omitempty"`
	CertificateStatus      *string    `json:"certificate_status,omitempty"`
	FailureReason          *string    `json:"failure_reason,omitempty"`
	VerifiedAt             *time.Time `json:"verified_at,omitempty"`
	ActivatedAt            *time.Time `json:"activated_at,omitempty"`
	LastCheckedAt          *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	DeletedAt              *time.Time `json:"-"`
	RoutingTarget          *string    `json:"routing_target,omitempty"`
	RoutingStatus          string     `json:"routing_status,omitempty"`
	RoutingCheckedAt       *time.Time `json:"routing_checked_at,omitempty"`
	RoutingError           *string    `json:"routing_error,omitempty"`
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
