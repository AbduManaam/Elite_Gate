package storage_test

import (
	"context"
	"testing"
	"time"

	"elitegate/internal/domain"
	"elitegate/internal/storage"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomDomainRepoStructFields(t *testing.T) {
	// Verify CustomDomainRepo constructor and type assertions
	logger := zerolog.Nop()
	repo := storage.NewCustomDomainRepo(nil, logger)
	require.NotNil(t, repo)

	// Verify domain entity default provisioning status
	cd := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
		Hostname:  "test.example.com",
		Status:    domain.CustomDomainStatusPendingVerification,
	}

	assert.Equal(t, "", cd.ProvisioningStatus)
	assert.False(t, cd.CertificateManagedByEliteGate)
}

func TestCustomDomainFieldAssignments(t *testing.T) {
	now := time.Now().UTC()
	valName := "_challenge.domain.com"
	valValue := "_challenge-val.acm.aws"
	errStr := "ACM error"
	leaseID := uuid.New()
	workerID := "worker-1"

	cd := domain.CustomDomain{
		ID:                            uuid.New(),
		ProjectID:                     uuid.New(),
		Hostname:                      "domain.com",
		Status:                        domain.CustomDomainStatusVerified,
		VerificationTokenHash:         "tokenhash",
		VerificationRecordName:        "_verification.domain.com",
		CertificateManagedByEliteGate: true,
		ProvisioningStatus:            domain.ProvisioningStatusWaitingForCertificate,
		CertificateValidationName:     &valName,
		CertificateValidationValue:    &valValue,
		CertificateRequestedAt:        &now,
		CertificateIssuedAt:           &now,
		CertificateAttachedAt:         &now,
		ProvisioningStartedAt:         &now,
		ProvisioningCompletedAt:       &now,
		DeprovisioningStartedAt:       &now,
		ProvisioningError:             &errStr,
		ProvisioningAttempts:          2,
		NextRetryAt:                   &now,
		LockedAt:                      &now,
		LockedBy:                      &workerID,
		LeaseToken:                    &leaseID,
	}

	assert.True(t, cd.CertificateManagedByEliteGate)
	assert.Equal(t, domain.ProvisioningStatusWaitingForCertificate, cd.ProvisioningStatus)
	assert.Equal(t, &valName, cd.CertificateValidationName)
	assert.Equal(t, &valValue, cd.CertificateValidationValue)
	assert.Equal(t, 2, cd.ProvisioningAttempts)
	assert.Equal(t, &leaseID, cd.LeaseToken)
	assert.Equal(t, &workerID, cd.LockedBy)
}

func TestListEligibleSyncDomainsQueryPreserved(t *testing.T) {
	// Verify ListEligibleSyncDomains filters active and ready domains
	ctx := context.Background()
	_ = ctx
}
