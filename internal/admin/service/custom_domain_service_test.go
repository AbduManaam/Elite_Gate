package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"elitegate/internal/domain"
	"elitegate/internal/storage"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDNSResolver struct {
	records   []string
	txtErr    error
	cnameHost string
	cnameErr  error
}

func (f *fakeDNSResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return f.records, f.txtErr
}

func (f *fakeDNSResolver) LookupCNAME(ctx context.Context, host string) (string, error) {
	return f.cnameHost, f.cnameErr
}

type fakeTXTResolver = fakeDNSResolver

type fakeCustomDomainRepo struct {
	domains           map[string]*domain.CustomDomain
	getByIDError      error
	markVerifiedError error
}

func newFakeCustomDomainRepo() *fakeCustomDomainRepo {
	return &fakeCustomDomainRepo{
		domains: make(map[string]*domain.CustomDomain),
	}
}

func (f *fakeCustomDomainRepo) HostnameExists(ctx context.Context, hostname string) (bool, error) {
	for _, d := range f.domains {
		if strings.EqualFold(d.Hostname, hostname) && d.DeletedAt == nil {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeCustomDomainRepo) Create(ctx context.Context, d *domain.CustomDomain) error {
	f.domains[d.ID.String()] = d
	return nil
}

func (f *fakeCustomDomainRepo) GetByIDForProject(ctx context.Context, id, projectID uuid.UUID) (*domain.CustomDomain, error) {
	if f.getByIDError != nil {
		return nil, f.getByIDError
	}
	d, ok := f.domains[id.String()]
	if !ok || d.ProjectID != projectID || d.DeletedAt != nil {
		return nil, storage.ErrCustomDomainNotFound
	}
	return d, nil
}

func (f *fakeCustomDomainRepo) MarkVerified(ctx context.Context, id, projectID uuid.UUID) (*domain.CustomDomain, error) {
	if f.markVerifiedError != nil {
		return nil, f.markVerifiedError
	}
	d, err := f.GetByIDForProject(ctx, id, projectID)
	if err != nil {
		return nil, err
	}
	d.Status = domain.CustomDomainStatusVerified
	now := time.Now()
	d.VerifiedAt = &now
	return d, nil
}

func (f *fakeCustomDomainRepo) ListByProject(ctx context.Context, projectID uuid.UUID) ([]domain.CustomDomain, error) {
	result := make([]domain.CustomDomain, 0)
	for _, d := range f.domains {
		if d.ProjectID == projectID && d.DeletedAt == nil {
			result = append(result, *d)
		}
	}
	return result, nil
}

func (f *fakeCustomDomainRepo) SoftDelete(ctx context.Context, id, projectID uuid.UUID) error {
	d, ok := f.domains[id.String()]
	if !ok || d.ProjectID != projectID || d.DeletedAt != nil {
		return storage.ErrCustomDomainNotFound
	}
	now := time.Now()
	d.DeletedAt = &now
	return nil
}

func (f *fakeCustomDomainRepo) RecordVerificationFailure(ctx context.Context, id, projectID uuid.UUID, reason string) error {
	d, err := f.GetByIDForProject(ctx, id, projectID)
	if err != nil {
		return err
	}
	now := time.Now()
	d.LastCheckedAt = &now
	d.FailureReason = &reason
	return nil
}

func (f *fakeCustomDomainRepo) UpdateRoutingStatus(ctx context.Context, id, projectID uuid.UUID, status string, target string, routingError *string) (*domain.CustomDomain, error) {
	d, err := f.GetByIDForProject(ctx, id, projectID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	d.RoutingStatus = status
	d.RoutingTarget = &target
	d.RoutingCheckedAt = &now
	d.RoutingError = routingError
	return d, nil
}

func (f *fakeCustomDomainRepo) MarkActive(ctx context.Context, id, projectID uuid.UUID) (*domain.CustomDomain, error) {
	d, err := f.GetByIDForProject(ctx, id, projectID)
	if err != nil {
		return nil, err
	}
	if d.Status != domain.CustomDomainStatusVerified || d.RoutingStatus != domain.CustomDomainRoutingStatusReady {
		return nil, storage.ErrCustomDomainNotFound
	}
	d.Status = domain.CustomDomainStatusActive
	now := time.Now()
	d.ActivatedAt = &now
	return d, nil
}

func TestVerifyCustomDomain_Success(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectID := uuid.New()
	rawToken := "test-verification-token-1234567890"
	tokenHash := hashCustomDomainVerificationToken(rawToken)

	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "api.example.com",
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  tokenHash,
		VerificationRecordName: "_elitegate-verification.api.example.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		records: []string{"elitegate-verification=" + rawToken},
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	result, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.CustomDomainStatusVerified, result.Status)
	assert.NotNil(t, result.VerifiedAt)
}

func TestVerifyCustomDomain_MultipleTXTRecords(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectID := uuid.New()
	rawToken := "valid-token-abc123456"
	tokenHash := hashCustomDomainVerificationToken(rawToken)

	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "app.acme.com",
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  tokenHash,
		VerificationRecordName: "_elitegate-verification.app.acme.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		records: []string{
			"v=spf1 include:_spf.google.com ~all",
			"some-other-verification=999",
			"elitegate-verification=" + rawToken,
		},
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	result, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	require.NoError(t, err)
	assert.Equal(t, domain.CustomDomainStatusVerified, result.Status)
}

func TestVerifyCustomDomain_UnrelatedTXTRecordsOnly(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectID := uuid.New()
	tokenHash := hashCustomDomainVerificationToken("expected-token")

	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "app.acme.com",
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  tokenHash,
		VerificationRecordName: "_elitegate-verification.app.acme.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		records: []string{
			"v=spf1 include:_spf.google.com ~all",
			"google-site-verification=abcdef",
		},
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	_, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrVerificationTokenMismatch)
}

func TestVerifyCustomDomain_RecordNotFound(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectID := uuid.New()
	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "missing.com",
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  "somehash",
		VerificationRecordName: "_elitegate-verification.missing.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		txtErr: &net.DNSError{IsNotFound: true},
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	_, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrVerificationRecordNotFound)
}

func TestVerifyCustomDomain_TokenMismatch(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectID := uuid.New()
	tokenHash := hashCustomDomainVerificationToken("correct-token")

	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "mismatch.com",
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  tokenHash,
		VerificationRecordName: "_elitegate-verification.mismatch.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		records: []string{"elitegate-verification=wrong-token"},
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	_, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrVerificationTokenMismatch)
}

func TestVerifyCustomDomain_DNSResolverFailure(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectID := uuid.New()
	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "dnserror.com",
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  "somehash",
		VerificationRecordName: "_elitegate-verification.dnserror.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		txtErr: errors.New("connection timed out"),
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	_, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "lookup TXT record")
}

func TestVerifyCustomDomain_DomainNotFound(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	resolver := &fakeTXTResolver{}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	_, err := svc.VerifyCustomDomain(context.Background(), uuid.New(), uuid.New())

	assert.ErrorIs(t, err, ErrCustomDomainNotFound)
}

func TestVerifyCustomDomain_AlreadyVerifiedIdempotent(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectID := uuid.New()
	now := time.Now()
	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "alreadyverified.com",
		Status:                 domain.CustomDomainStatusVerified,
		VerificationTokenHash:  "hash",
		VerificationRecordName: "_elitegate-verification.alreadyverified.com",
		VerifiedAt:             &now,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		txtErr: &net.DNSError{IsNotFound: true}, // Resolver won't even be called
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	result, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	require.NoError(t, err)
	assert.Equal(t, domain.CustomDomainStatusVerified, result.Status)
}

func TestVerifyCustomDomain_ActiveDomainIdempotent(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectID := uuid.New()
	now := time.Now()
	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "active.com",
		Status:                 domain.CustomDomainStatusActive,
		VerificationTokenHash:  "hash",
		VerificationRecordName: "_elitegate-verification.active.com",
		VerifiedAt:             &now,
		ActivatedAt:            &now,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	result, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	require.NoError(t, err)
	assert.Equal(t, domain.CustomDomainStatusActive, result.Status)
}

func TestVerifyCustomDomain_TenantIsolation(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectA := uuid.New()
	projectB := uuid.New()

	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectA,
		Hostname:               "tenant-a.com",
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  "hash",
		VerificationRecordName: "_elitegate-verification.tenant-a.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		records: []string{"elitegate-verification=hash"},
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	// Tenant B attempts to verify Tenant A's domain
	_, err := svc.VerifyCustomDomain(context.Background(), projectB, d.ID)

	assert.ErrorIs(t, err, ErrCustomDomainNotFound)
}

func TestVerifyCustomDomain_MarkVerifiedFailure(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	repo.markVerifiedError = errors.New("db error")

	projectID := uuid.New()
	rawToken := "token123"
	tokenHash := hashCustomDomainVerificationToken(rawToken)

	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "dberror.com",
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  tokenHash,
		VerificationRecordName: "_elitegate-verification.dberror.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		records: []string{"elitegate-verification=" + rawToken},
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	_, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mark custom domain verified")
}

func TestSecureHashEqual(t *testing.T) {
	hash1 := hashCustomDomainVerificationToken("token1")
	hash2 := hashCustomDomainVerificationToken("token1")
	hash3 := hashCustomDomainVerificationToken("token2")

	assert.True(t, secureHashEqual(hash1, hash2))
	assert.False(t, secureHashEqual(hash1, hash3))
	assert.False(t, secureHashEqual(hash1, "short"))
}

func TestVerifyCustomDomain_FailureReasonRecordedOnMismatch(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectID := uuid.New()
	tokenHash := hashCustomDomainVerificationToken("expected-token")

	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "recorded-mismatch.com",
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  tokenHash,
		VerificationRecordName: "_elitegate-verification.recorded-mismatch.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		records: []string{"elitegate-verification=wrong-token"},
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	_, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrVerificationTokenMismatch)
	stored, getErr := repo.GetByIDForProject(context.Background(), d.ID, projectID)
	require.NoError(t, getErr)
	assert.NotNil(t, stored.LastCheckedAt)
	assert.NotNil(t, stored.FailureReason)
	assert.Equal(t, "DNS verification token does not match", *stored.FailureReason)
}

func TestVerifyCustomDomain_FailureReasonRecordedOnMissingRecord(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	projectID := uuid.New()
	d := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               "recorded-missing.com",
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  "hash",
		VerificationRecordName: "_elitegate-verification.recorded-missing.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeTXTResolver{
		txtErr: &net.DNSError{IsNotFound: true},
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, "gateway.elitegateway.site", logger)
	_, err := svc.VerifyCustomDomain(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrVerificationRecordNotFound)
	stored, getErr := repo.GetByIDForProject(context.Background(), d.ID, projectID)
	require.NoError(t, getErr)
	assert.NotNil(t, stored.LastCheckedAt)
	assert.NotNil(t, stored.FailureReason)
	assert.Equal(t, "DNS verification TXT record missing", *stored.FailureReason)
}

func TestListCustomDomains_Success(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()

	d1 := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "a.example.com",
		Status:    domain.CustomDomainStatusPendingVerification,
	}
	d2 := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "b.example.com",
		Status:    domain.CustomDomainStatusVerified,
	}
	require.NoError(t, repo.Create(context.Background(), d1))
	require.NoError(t, repo.Create(context.Background(), d2))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	domains, err := svc.ListCustomDomains(context.Background(), projectID)

	require.NoError(t, err)
	assert.Len(t, domains, 2)
}

func TestListCustomDomains_EmptyList(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	domains, err := svc.ListCustomDomains(context.Background(), projectID)

	require.NoError(t, err)
	assert.NotNil(t, domains)
	assert.Len(t, domains, 0)
}

func TestListCustomDomains_ExcludesDeleted(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()

	d1 := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "active.example.com",
	}
	d2 := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "deleted.example.com",
	}
	require.NoError(t, repo.Create(context.Background(), d1))
	require.NoError(t, repo.Create(context.Background(), d2))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	require.NoError(t, svc.DeleteCustomDomain(context.Background(), projectID, d2.ID))

	domains, err := svc.ListCustomDomains(context.Background(), projectID)
	require.NoError(t, err)
	assert.Len(t, domains, 1)
	assert.Equal(t, d1.ID, domains[0].ID)
}

func TestGetCustomDomain_Success(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "get.example.com",
		Status:    domain.CustomDomainStatusVerified,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	result, err := svc.GetCustomDomain(context.Background(), projectID, d.ID)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, d.ID, result.ID)
	assert.Equal(t, "get.example.com", result.Hostname)
}

func TestGetCustomDomain_NotFound(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	_, err := svc.GetCustomDomain(context.Background(), uuid.New(), uuid.New())

	assert.ErrorIs(t, err, ErrCustomDomainNotFound)
}

func TestGetCustomDomain_WrongProject(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectA := uuid.New()
	projectB := uuid.New()

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectA,
		Hostname:  "projecta.example.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	_, err := svc.GetCustomDomain(context.Background(), projectB, d.ID)

	assert.ErrorIs(t, err, ErrCustomDomainNotFound)
}

func TestDeleteCustomDomain_Success(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "del.example.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	err := svc.DeleteCustomDomain(context.Background(), projectID, d.ID)
	require.NoError(t, err)

	_, getErr := svc.GetCustomDomain(context.Background(), projectID, d.ID)
	assert.ErrorIs(t, getErr, ErrCustomDomainNotFound)
}

func TestDeleteCustomDomain_DoubleDelete(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "double-del.example.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	require.NoError(t, svc.DeleteCustomDomain(context.Background(), projectID, d.ID))

	// Second delete attempt must return ErrCustomDomainNotFound (404)
	err := svc.DeleteCustomDomain(context.Background(), projectID, d.ID)
	assert.ErrorIs(t, err, ErrCustomDomainNotFound)
}

func TestDeleteCustomDomain_WrongProject(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectA := uuid.New()
	projectB := uuid.New()

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectA,
		Hostname:  "proj-a.example.com",
	}
	require.NoError(t, repo.Create(context.Background(), d))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	err := svc.DeleteCustomDomain(context.Background(), projectB, d.ID)
	assert.ErrorIs(t, err, ErrCustomDomainNotFound)

	// Domain remains intact for project A
	stored, getErr := svc.GetCustomDomain(context.Background(), projectA, d.ID)
	require.NoError(t, getErr)
	assert.Equal(t, d.ID, stored.ID)
}

func TestVerificationTokenHashNeverReturnedInJSON(t *testing.T) {
	d := &domain.CustomDomain{
		ID:                    uuid.New(),
		ProjectID:             uuid.New(),
		Hostname:              "secure.example.com",
		Status:                domain.CustomDomainStatusVerified,
		VerificationTokenHash: "super-secret-hash-12345",
	}

	data, err := json.Marshal(d)
	require.NoError(t, err)

	jsonStr := string(data)
	assert.NotContains(t, jsonStr, "verification_token_hash")
	assert.NotContains(t, jsonStr, "super-secret-hash-12345")
	assert.NotContains(t, jsonStr, "deleted_at")
}

func TestCheckCustomDomainRouting_VerifiedCorrectCNAME(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()
	targetHost := "gateway.elitegateway.site"

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "api.example.com",
		Status:    domain.CustomDomainStatusVerified,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeDNSResolver{
		cnameHost: "gateway.elitegateway.site.",
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, targetHost, logger)
	result, err := svc.CheckCustomDomainRouting(context.Background(), projectID, d.ID)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.CustomDomainRoutingStatusReady, result.RoutingStatus)
	assert.Equal(t, targetHost, *result.RoutingTarget)
	assert.Nil(t, result.RoutingError)
	assert.NotNil(t, result.RoutingCheckedAt)
}

func TestCheckCustomDomainRouting_VerifiedIncorrectCNAME(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()
	targetHost := "gateway.elitegateway.site"

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "mismatch.example.com",
		Status:    domain.CustomDomainStatusVerified,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeDNSResolver{
		cnameHost: "wrong.target.com.",
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, targetHost, logger)
	_, err := svc.CheckCustomDomainRouting(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrCNAMERoutingMismatch)

	stored, getErr := repo.GetByIDForProject(context.Background(), d.ID, projectID)
	require.NoError(t, getErr)
	assert.Equal(t, domain.CustomDomainRoutingStatusFailed, stored.RoutingStatus)
	assert.NotNil(t, stored.RoutingError)
	assert.Contains(t, *stored.RoutingError, "expected gateway.elitegateway.site, got wrong.target.com")
}

func TestCheckCustomDomainRouting_VerifiedMissingCNAME(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()
	targetHost := "gateway.elitegateway.site"

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "missing.example.com",
		Status:    domain.CustomDomainStatusVerified,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeDNSResolver{
		cnameErr: &net.DNSError{IsNotFound: true},
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, targetHost, logger)
	_, err := svc.CheckCustomDomainRouting(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrCNAMERecordNotFound)

	stored, getErr := repo.GetByIDForProject(context.Background(), d.ID, projectID)
	require.NoError(t, getErr)
	assert.Equal(t, domain.CustomDomainRoutingStatusFailed, stored.RoutingStatus)
	assert.NotNil(t, stored.RoutingError)
}

func TestCheckCustomDomainRouting_PendingTXTVerification(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()
	targetHost := "gateway.elitegateway.site"

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "unverified.example.com",
		Status:    domain.CustomDomainStatusPendingVerification,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeDNSResolver{
		cnameHost: targetHost,
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, targetHost, logger)
	_, err := svc.CheckCustomDomainRouting(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrCustomDomainNotVerified)
}

func TestCheckCustomDomainRouting_WrongProject(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectA := uuid.New()
	projectB := uuid.New()
	targetHost := "gateway.elitegateway.site"

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectA,
		Hostname:  "projecta.example.com",
		Status:    domain.CustomDomainStatusVerified,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeDNSResolver{
		cnameHost: targetHost,
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, targetHost, logger)
	_, err := svc.CheckCustomDomainRouting(context.Background(), projectB, d.ID)

	assert.ErrorIs(t, err, ErrCustomDomainNotFound)
}

func TestCheckCustomDomainRouting_RepeatedCheckIdempotent(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()
	targetHost := "gateway.elitegateway.site"

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "idempotent.example.com",
		Status:    domain.CustomDomainStatusVerified,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeDNSResolver{
		cnameHost: targetHost,
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, targetHost, logger)

	// First check -> ready
	result1, err1 := svc.CheckCustomDomainRouting(context.Background(), projectID, d.ID)
	require.NoError(t, err1)
	assert.Equal(t, domain.CustomDomainRoutingStatusReady, result1.RoutingStatus)

	// Second check -> remains ready, returns 200 OK without error
	result2, err2 := svc.CheckCustomDomainRouting(context.Background(), projectID, d.ID)
	require.NoError(t, err2)
	assert.Equal(t, domain.CustomDomainRoutingStatusReady, result2.RoutingStatus)
}

func TestCheckCustomDomainRouting_Normalization(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()
	targetHost := "GATEWAY.ELITEGATEWAY.SITE"

	d := &domain.CustomDomain{
		ID:        uuid.New(),
		ProjectID: projectID,
		Hostname:  "norm.example.com",
		Status:    domain.CustomDomainStatusVerified,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	resolver := &fakeDNSResolver{
		cnameHost: "gateway.elitegateway.site.", // trailing dot & lowercase
	}

	svc := NewCustomDomainServiceWithResolver(repo, resolver, targetHost, logger)
	result, err := svc.CheckCustomDomainRouting(context.Background(), projectID, d.ID)

	require.NoError(t, err)
	assert.Equal(t, domain.CustomDomainRoutingStatusReady, result.RoutingStatus)
}

func TestActivateCustomDomain_VerifiedAndReady_Success(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()

	d := &domain.CustomDomain{
		ID:            uuid.New(),
		ProjectID:     projectID,
		Hostname:      "activate.example.com",
		Status:        domain.CustomDomainStatusVerified,
		RoutingStatus: domain.CustomDomainRoutingStatusReady,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	result, err := svc.ActivateCustomDomain(context.Background(), projectID, d.ID)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.CustomDomainStatusActive, result.Status)
	assert.NotNil(t, result.ActivatedAt)
}

func TestActivateCustomDomain_AlreadyActiveAndReady_Idempotent(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()
	now := time.Now()

	d := &domain.CustomDomain{
		ID:            uuid.New(),
		ProjectID:     projectID,
		Hostname:      "idempotent.example.com",
		Status:        domain.CustomDomainStatusActive,
		RoutingStatus: domain.CustomDomainRoutingStatusReady,
		ActivatedAt:   &now,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	result, err := svc.ActivateCustomDomain(context.Background(), projectID, d.ID)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.CustomDomainStatusActive, result.Status)
}

func TestActivateCustomDomain_RoutingNotReady(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()

	d := &domain.CustomDomain{
		ID:            uuid.New(),
		ProjectID:     projectID,
		Hostname:      "routingpending.example.com",
		Status:        domain.CustomDomainStatusVerified,
		RoutingStatus: domain.CustomDomainRoutingStatusPending,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	_, err := svc.ActivateCustomDomain(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrCustomDomainRoutingNotReady)
}

func TestActivateCustomDomain_PendingVerification(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()

	d := &domain.CustomDomain{
		ID:            uuid.New(),
		ProjectID:     projectID,
		Hostname:      "unverified.example.com",
		Status:        domain.CustomDomainStatusPendingVerification,
		RoutingStatus: domain.CustomDomainRoutingStatusReady,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	_, err := svc.ActivateCustomDomain(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrCustomDomainNotVerified)
}

func TestActivateCustomDomain_DeletedDomain(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectID := uuid.New()

	d := &domain.CustomDomain{
		ID:            uuid.New(),
		ProjectID:     projectID,
		Hostname:      "deleted.example.com",
		Status:        domain.CustomDomainStatusVerified,
		RoutingStatus: domain.CustomDomainRoutingStatusReady,
	}
	require.NoError(t, repo.Create(context.Background(), d))
	require.NoError(t, repo.SoftDelete(context.Background(), d.ID, projectID))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	_, err := svc.ActivateCustomDomain(context.Background(), projectID, d.ID)

	assert.ErrorIs(t, err, ErrCustomDomainNotFound)
}

func TestActivateCustomDomain_WrongProject(t *testing.T) {
	logger := zerolog.Nop()
	repo := newFakeCustomDomainRepo()
	projectA := uuid.New()
	projectB := uuid.New()

	d := &domain.CustomDomain{
		ID:            uuid.New(),
		ProjectID:     projectA,
		Hostname:      "projecta.example.com",
		Status:        domain.CustomDomainStatusVerified,
		RoutingStatus: domain.CustomDomainRoutingStatusReady,
	}
	require.NoError(t, repo.Create(context.Background(), d))

	svc := NewCustomDomainService(repo, nil, "gateway.elitegateway.site", logger)
	_, err := svc.ActivateCustomDomain(context.Background(), projectB, d.ID)

	assert.ErrorIs(t, err, ErrCustomDomainNotFound)
}
