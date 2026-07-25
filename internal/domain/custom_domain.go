package domain

import (
	"time"

	"github.com/google/uuid"
)

// CustomDomain represents a custom domain entity in the system.
type CustomDomain struct {
	ID                     uuid.UUID `json:"id"`
	ProjectID              uuid.UUID `json:"project_id"`
	Hostname               string    `json:"hostname"`
	Status                 string    `json:"status"`
	VerificationTokenHash  string    `json:"verification_token_hash,omitempty"`
	VerificationRecordName string    `json:"verification_record_name,omitempty"`
	CertificateARN         *string   `json:"certificate_arn,omitempty"`
	CertificateStatus      *string   `json:"certificate_status,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// VerificationRecord holds details for DNS verification of a custom domain.
type VerificationRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CreateCustomDomainRequest payload for creating a custom domain.
type CreateCustomDomainRequest struct {
	Hostname string `json:"hostname" binding:"required"`
}

// CreateCustomDomainResponse payload returned after creating a custom domain.
type CreateCustomDomainResponse struct {
	ID                 uuid.UUID          `json:"id"`
	Hostname           string             `json:"hostname"`
	Status             string             `json:"status"`
	VerificationRecord VerificationRecord `json:"verification_record"`
}
