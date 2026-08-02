package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"elitegate/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomDomainModelFieldsAndConstants(t *testing.T) {
	// Verify ProvisioningStatus constants
	assert.Equal(t, "not_started", domain.ProvisioningStatusNotStarted)
	assert.Equal(t, "requesting_certificate", domain.ProvisioningStatusRequestingCertificate)
	assert.Equal(t, "waiting_for_validation_record", domain.ProvisioningStatusWaitingForValidationRecord)
	assert.Equal(t, "waiting_for_dns", domain.ProvisioningStatusWaitingForDNS)
	assert.Equal(t, "waiting_for_certificate", domain.ProvisioningStatusWaitingForCertificate)
	assert.Equal(t, "attaching_certificate", domain.ProvisioningStatusAttachingCertificate)
	assert.Equal(t, "completed", domain.ProvisioningStatusCompleted)
	assert.Equal(t, "failed", domain.ProvisioningStatusFailed)
	assert.Equal(t, "deprovisioning", domain.ProvisioningStatusDeprovisioning)
	assert.Equal(t, "deprovisioned", domain.ProvisioningStatusDeprovisioned)
	assert.Equal(t, "deprovision_failed", domain.ProvisioningStatusDeprovisionFailed)

	// Verify CustomDomain struct mapping and JSON serialization
	id := uuid.New()
	projectID := uuid.New()
	now := time.Now().UTC()
	valName := "_acm-challenge.example.com"
	valValue := "_acm-value.aws"

	cd := domain.CustomDomain{
		ID:                            id,
		ProjectID:                     projectID,
		Hostname:                      "app.example.com",
		Status:                        domain.CustomDomainStatusVerified,
		VerificationTokenHash:         "hash123",
		VerificationRecordName:        "_elitegate-verification.app.example.com",
		RoutingTarget:                 nil,
		RoutingStatus:                 domain.CustomDomainRoutingStatusReady,
		CertificateManagedByEliteGate: true,
		ProvisioningStatus:            domain.ProvisioningStatusWaitingForDNS,
		CertificateValidationName:     &valName,
		CertificateValidationValue:    &valValue,
		CertificateRequestedAt:        &now,
		CreatedAt:                     now,
		UpdatedAt:                     now,
	}

	assert.Equal(t, id, cd.ID)
	assert.Equal(t, projectID, cd.ProjectID)
	assert.Equal(t, "app.example.com", cd.Hostname)
	assert.Equal(t, true, cd.CertificateManagedByEliteGate)
	assert.Equal(t, domain.ProvisioningStatusWaitingForDNS, cd.ProvisioningStatus)

	// Verify JSON marshaling excludes sensitive fields like VerificationTokenHash, LockedAt, LockedBy, LeaseToken
	jsonData, err := json.Marshal(cd)
	require.NoError(t, err)

	jsonStr := string(jsonData)
	assert.Contains(t, jsonStr, `"certificate_managed_by_elitegate":true`)
	assert.Contains(t, jsonStr, `"provisioning_status":"waiting_for_dns"`)
	assert.Contains(t, jsonStr, `"certificate_validation_name":"_acm-challenge.example.com"`)
	assert.NotContains(t, jsonStr, `verification_token_hash`)
	assert.NotContains(t, jsonStr, `lease_token`)
}

func TestProvisioningStatusDTO(t *testing.T) {
	id := uuid.New()
	valName := "_acm.example.com"
	valValue := "_value.acm.aws"

	dto := domain.ProvisioningStatusDTO{
		ID:                         id,
		Hostname:                   "sub.example.com",
		Status:                     domain.CustomDomainStatusVerified,
		RoutingStatus:              domain.CustomDomainRoutingStatusReady,
		ProvisioningStatus:         domain.ProvisioningStatusWaitingForCertificate,
		CertificateValidationName:  &valName,
		CertificateValidationValue: &valValue,
	}

	jsonData, err := json.Marshal(dto)
	require.NoError(t, err)

	assert.Contains(t, string(jsonData), `"provisioning_status":"waiting_for_certificate"`)
	assert.Contains(t, string(jsonData), `"certificate_validation_name":"_acm.example.com"`)
}
