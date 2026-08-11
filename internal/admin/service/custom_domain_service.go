package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"

	"elitegate/internal/domain"
	domainhelper "elitegate/internal/helper"
	"elitegate/internal/storage"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const (
	customDomainVerificationTokenBytes = 32
	customDomainVerificationPrefix     = "elitegate-verification="
	customDomainVerificationRecord     = "_elitegate-verification"
)

var (
	ErrInvalidCustomDomainHostname = errors.New(
		"invalid custom domain hostname",
	)
	ErrCustomDomainAlreadyExists = errors.New(
		"custom domain already exists",
	)
	ErrCustomDomainNotFound = errors.New(
		"custom domain not found",
	)
	ErrVerificationRecordNotFound = errors.New(
		"DNS verification record not found",
	)
	ErrVerificationTokenMismatch = errors.New(
		"DNS verification token does not match",
	)
	ErrCustomDomainNotVerified = errors.New(
		"custom domain ownership must be verified before checking routing",
	)
	ErrCNAMERecordNotFound = errors.New(
		"DNS CNAME record not found",
	)
	ErrCNAMERoutingMismatch = errors.New(
		"CNAME record does not point to the expected gateway target",
	)
	ErrCustomDomainRoutingNotReady = errors.New(
		"custom domain routing status must be ready before activation",
	)
	ErrAutomationDisabled = errors.New(
		"automatic certificate provisioning is currently disabled",
	)
	ErrDomainNotEligibleForRetry = errors.New(
		"custom domain not eligible for retry",
	)
)

// CustomDomainRepository defines the storage operations required by CustomDomainService.
type CustomDomainRepository interface {
	HostnameExists(
		ctx context.Context,
		hostname string,
	) (bool, error)

	Create(
		ctx context.Context,
		customDomain *domain.CustomDomain,
	) error

	GetByIDForProject(
		ctx context.Context,
		id uuid.UUID,
		projectID uuid.UUID,
	) (*domain.CustomDomain, error)

	ListByProject(
		ctx context.Context,
		projectID uuid.UUID,
	) ([]domain.CustomDomain, error)

	MarkVerified(
		ctx context.Context,
		id uuid.UUID,
		projectID uuid.UUID,
	) (*domain.CustomDomain, error)

	MarkActive(
		ctx context.Context,
		id uuid.UUID,
		projectID uuid.UUID,
	) (*domain.CustomDomain, error)

	RecordVerificationFailure(
		ctx context.Context,
		id uuid.UUID,
		projectID uuid.UUID,
		reason string,
	) error

	SoftDelete(
		ctx context.Context,
		id uuid.UUID,
		projectID uuid.UUID,
	) error

	UpdateRoutingStatus(
		ctx context.Context,
		id uuid.UUID,
		projectID uuid.UUID,
		status string,
		target string,
		routingError *string,
	) (*domain.CustomDomain, error)

	EnqueueProvisioning(
		ctx context.Context,
		id uuid.UUID,
		projectID uuid.UUID,
	) (*domain.CustomDomain, error)

	ResetProvisioningForRetry(
		ctx context.Context,
		id uuid.UUID,
		projectID uuid.UUID,
		targetStatus string,
	) (*domain.CustomDomain, error)

	EnqueueDeprovisioning(
		ctx context.Context,
		id uuid.UUID,
		projectID uuid.UUID,
	) (*domain.CustomDomain, error)

	MarkDeprovisionFailed(
		ctx context.Context,
		id uuid.UUID,
		leaseToken uuid.UUID,
		errStr string,
	) error

	ResetDeprovisioningForRetry(
		ctx context.Context,
		id uuid.UUID,
		projectID uuid.UUID,
	) (*domain.CustomDomain, error)

	GetActiveProjectGatewayIngress(
		ctx context.Context,
		projectID uuid.UUID,
	) (*storage.ProjectGatewayIngress, error)
}

// DNSResolver represents a DNS resolver capable of querying TXT and CNAME records.
//
// Using an interface allows the DNS behavior to be mocked in unit tests.
type DNSResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
}

// ActivateCustomDomainResult represents the result of an activation attempt.
type ActivateCustomDomainResult struct {
	Domain *domain.CustomDomain
	State  domain.ActivationState
}

// CustomDomainService handles custom-domain business logic.
type CustomDomainService struct {
	repo              CustomDomainRepository
	resolver          DNSResolver
	gatewayPublicHost string
	automationEnabled bool
	logger            zerolog.Logger
}

// NewCustomDomainService creates a CustomDomainService with injected DNSResolver.
func NewCustomDomainService(
	repo CustomDomainRepository,
	resolver DNSResolver,
	gatewayPublicHost string,
	logger zerolog.Logger,
) *CustomDomainService {
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	return &CustomDomainService{
		repo:              repo,
		resolver:          resolver,
		gatewayPublicHost: gatewayPublicHost,
		automationEnabled: false,
		logger: logger.With().
			Str("service", "custom_domain").
			Logger(),
	}
}

// NewCustomDomainServiceWithAutomation creates a CustomDomainService with explicit automation setting.
func NewCustomDomainServiceWithAutomation(
	repo CustomDomainRepository,
	resolver DNSResolver,
	gatewayPublicHost string,
	automationEnabled bool,
	logger zerolog.Logger,
) *CustomDomainService {
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	return &CustomDomainService{
		repo:              repo,
		resolver:          resolver,
		gatewayPublicHost: gatewayPublicHost,
		automationEnabled: automationEnabled,
		logger: logger.With().
			Str("service", "custom_domain").
			Logger(),
	}
}

// NewCustomDomainServiceWithResolver creates a CustomDomainService with a
// custom DNS resolver. This is mainly useful for unit tests.
func NewCustomDomainServiceWithResolver(
	repo CustomDomainRepository,
	resolver DNSResolver,
	gatewayPublicHost string,
	logger zerolog.Logger,
) *CustomDomainService {
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	return &CustomDomainService{
		repo:              repo,
		resolver:          resolver,
		gatewayPublicHost: gatewayPublicHost,
		automationEnabled: false,
		logger: logger.With().
			Str("service", "custom_domain").
			Logger(),
	}
}

// CreateCustomDomain registers a new custom domain for a project.
//
// The plain verification token is returned only in this response.
// Only its SHA-256 hash is stored in PostgreSQL.
func (s *CustomDomainService) CreateCustomDomain(
	ctx context.Context,
	projectID uuid.UUID,
	rawHostname string,
) (*domain.CreateCustomDomainResponse, error) {
	hostname, err := domainhelper.NormalizeHostname(rawHostname)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %v",
			ErrInvalidCustomDomainHostname,
			err,
		)
	}

	exists, err := s.repo.HostnameExists(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf(
			"check hostname availability: %w",
			err,
		)
	}

	if exists {
		return nil, ErrCustomDomainAlreadyExists
	}

	verificationToken, err := generateCustomDomainVerificationToken()
	if err != nil {
		return nil, fmt.Errorf(
			"generate verification token: %w",
			err,
		)
	}

	verificationTokenHash := hashCustomDomainVerificationToken(
		verificationToken,
	)

	verificationRecordName := fmt.Sprintf(
		"%s.%s",
		customDomainVerificationRecord,
		hostname,
	)

	customDomain := &domain.CustomDomain{
		ID:                     uuid.New(),
		ProjectID:              projectID,
		Hostname:               hostname,
		Status:                 domain.CustomDomainStatusPendingVerification,
		VerificationTokenHash:  verificationTokenHash,
		VerificationRecordName: verificationRecordName,
		RoutingTarget:          &s.gatewayPublicHost,
		RoutingStatus:          domain.CustomDomainRoutingStatusPending,
	}

	if err := s.repo.Create(ctx, customDomain); err != nil {
		if errors.Is(err, storage.ErrHostnameConflict) {
			return nil, ErrCustomDomainAlreadyExists
		}

		return nil, fmt.Errorf(
			"create custom domain: %w",
			err,
		)
	}

	verificationValue := customDomainVerificationPrefix +
		verificationToken

	response := &domain.CreateCustomDomainResponse{
		ID:        customDomain.ID,
		ProjectID: customDomain.ProjectID,
		Hostname:  customDomain.Hostname,
		Status:    customDomain.Status,
		VerificationRecord: domain.VerificationRecord{
			Type:  "TXT",
			Name:  customDomain.VerificationRecordName,
			Value: verificationValue,
		},
		CreatedAt: customDomain.CreatedAt,
	}

	s.logger.Info().
		Str("custom_domain_id", customDomain.ID.String()).
		Str("project_id", customDomain.ProjectID.String()).
		Str("hostname", customDomain.Hostname).
		Msg("custom domain registration created")

	return response, nil
}

// VerifyCustomDomain checks the customer's DNS TXT record and marks the domain
// as verified when a matching verification token is found.
func (s *CustomDomainService) VerifyCustomDomain(
	ctx context.Context,
	projectID uuid.UUID,
	customDomainID uuid.UUID,
) (*domain.CustomDomain, error) {
	customDomain, err := s.repo.GetByIDForProject(
		ctx,
		customDomainID,
		projectID,
	)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}

		return nil, fmt.Errorf(
			"load custom domain for verification: %w",
			err,
		)
	}

	// Make verification idempotent. Calling the endpoint again after a
	// successful verification returns the existing domain.
	if customDomain.Status == domain.CustomDomainStatusVerified ||
		customDomain.Status == domain.CustomDomainStatusActive {
		return customDomain, nil
	}

	txtRecords, err := s.resolver.LookupTXT(
		ctx,
		customDomain.VerificationRecordName,
	)
	if err != nil {
		var dnsErr *net.DNSError

		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			recordErr := s.repo.RecordVerificationFailure(
				ctx,
				customDomain.ID,
				customDomain.ProjectID,
				"DNS verification TXT record missing",
			)
			if recordErr != nil {
				s.logger.Error().
					Err(recordErr).
					Str("custom_domain_id", customDomain.ID.String()).
					Msg("failed to record domain verification failure")
			}

			return nil, ErrVerificationRecordNotFound
		}

		return nil, fmt.Errorf(
			"lookup TXT record %q: %w",
			customDomain.VerificationRecordName,
			err,
		)
	}

	if !containsMatchingVerificationToken(
		txtRecords,
		customDomain.VerificationTokenHash,
	) {
		recordErr := s.repo.RecordVerificationFailure(
			ctx,
			customDomain.ID,
			customDomain.ProjectID,
			"DNS verification token does not match",
		)
		if recordErr != nil {
			s.logger.Error().
				Err(recordErr).
				Str("custom_domain_id", customDomain.ID.String()).
				Msg("failed to record domain verification failure")
		}

		return nil, ErrVerificationTokenMismatch
	}

	verifiedDomain, err := s.repo.MarkVerified(
		ctx,
		customDomain.ID,
		customDomain.ProjectID,
	)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}

		return nil, fmt.Errorf(
			"mark custom domain verified: %w",
			err,
		)
	}

	s.logger.Info().
		Str("custom_domain_id", verifiedDomain.ID.String()).
		Str("project_id", verifiedDomain.ProjectID.String()).
		Str("hostname", verifiedDomain.Hostname).
		Msg("custom domain DNS ownership verified")

	return verifiedDomain, nil
}

// containsMatchingVerificationToken checks all TXT records for the expected
// EliteGate verification token.
func containsMatchingVerificationToken(
	txtRecords []string,
	expectedTokenHash string,
) bool {
	for _, txtRecord := range txtRecords {
		txtRecord = strings.TrimSpace(txtRecord)

		if !strings.HasPrefix(
			txtRecord,
			customDomainVerificationPrefix,
		) {
			continue
		}

		token := strings.TrimSpace(
			strings.TrimPrefix(
				txtRecord,
				customDomainVerificationPrefix,
			),
		)

		if token == "" {
			continue
		}

		receivedTokenHash := hashCustomDomainVerificationToken(token)

		if secureHashEqual(
			receivedTokenHash,
			expectedTokenHash,
		) {
			return true
		}
	}

	return false
}

// secureHashEqual compares hashes in constant time.
func secureHashEqual(first string, second string) bool {
	if len(first) != len(second) {
		return false
	}

	return subtle.ConstantTimeCompare(
		[]byte(first),
		[]byte(second),
	) == 1
}

// generateCustomDomainVerificationToken generates a secure URL-safe token.
//
// A 32-byte random value provides 256 bits of randomness.
func generateCustomDomainVerificationToken() (string, error) {
	randomBytes := make(
		[]byte,
		customDomainVerificationTokenBytes,
	)

	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf(
			"read secure random bytes: %w",
			err,
		)
	}

	return base64.RawURLEncoding.EncodeToString(
		randomBytes,
	), nil
}

// hashCustomDomainVerificationToken creates the hash stored in PostgreSQL.
//
// The plain token must never be stored in the database.
func hashCustomDomainVerificationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ListCustomDomains returns all active (non-deleted) custom domains for a project.
func (s *CustomDomainService) ListCustomDomains(
	ctx context.Context,
	projectID uuid.UUID,
) ([]domain.CustomDomain, error) {
	customDomains, err := s.repo.ListByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list custom domains: %w", err)
	}

	return customDomains, nil
}

// GetCustomDomain retrieves a single active custom domain by ID and project ID.
func (s *CustomDomainService) GetCustomDomain(
	ctx context.Context,
	projectID uuid.UUID,
	customDomainID uuid.UUID,
) (*domain.CustomDomain, error) {
	customDomain, err := s.repo.GetByIDForProject(
		ctx,
		customDomainID,
		projectID,
	)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}

		return nil, fmt.Errorf("get custom domain: %w", err)
	}

	return customDomain, nil
}

// DeleteCustomDomain initiates asynchronous custom domain deprovisioning and cleanup.
// If the domain is already deprovisioned or soft-deleted, it idempotently returns success.
func (s *CustomDomainService) DeleteCustomDomain(
	ctx context.Context,
	projectID uuid.UUID,
	customDomainID uuid.UUID,
) (*domain.CustomDomain, error) {
	if !s.automationEnabled {
		return nil, ErrAutomationDisabled
	}

	cd, err := s.repo.EnqueueDeprovisioning(ctx, customDomainID, projectID)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}
		return nil, fmt.Errorf("enqueue deprovisioning: %w", err)
	}

	s.logger.Info().
		Str("custom_domain_id", cd.ID.String()).
		Str("project_id", cd.ProjectID.String()).
		Str("hostname", cd.Hostname).
		Str("provisioning_status", cd.ProvisioningStatus).
		Msg("custom domain deprovisioning enqueued")

	return cd, nil
}

// RetryDeprovisioning safely restarts custom domain deprovisioning after a failure.
func (s *CustomDomainService) RetryDeprovisioning(
	ctx context.Context,
	projectID uuid.UUID,
	customDomainID uuid.UUID,
) (*domain.CustomDomain, error) {
	if !s.automationEnabled {
		return nil, ErrAutomationDisabled
	}

	cd, err := s.repo.GetByIDForProject(ctx, customDomainID, projectID)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}
		return nil, fmt.Errorf("get custom domain for retry deprovisioning: %w", err)
	}

	if cd.ProvisioningStatus != domain.ProvisioningStatusDeprovisionFailed {
		return nil, ErrDomainNotEligibleForRetry
	}

	retried, err := s.repo.ResetDeprovisioningForRetry(ctx, customDomainID, projectID)
	if err != nil {
		return nil, fmt.Errorf("reset deprovisioning for retry: %w", err)
	}

	s.logger.Info().
		Str("custom_domain_id", retried.ID.String()).
		Str("project_id", retried.ProjectID.String()).
		Str("hostname", retried.Hostname).
		Msg("custom domain deprovisioning retry enqueued")

	return retried, nil
}

// CheckCustomDomainRouting verifies that the custom domain's CNAME record points to the gateway target.
func (s *CustomDomainService) CheckCustomDomainRouting(
	ctx context.Context,
	projectID uuid.UUID,
	customDomainID uuid.UUID,
) (*domain.CustomDomain, error) {
	customDomain, err := s.repo.GetByIDForProject(ctx, customDomainID, projectID)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}
		return nil, fmt.Errorf("load custom domain for routing check: %w", err)
	}

	if customDomain.Status != domain.CustomDomainStatusVerified &&
		customDomain.Status != domain.CustomDomainStatusActive {
		return nil, ErrCustomDomainNotVerified
	}

	expectedTarget := normalizeCNAME(s.gatewayPublicHost)

	resolvedCNAME, err := s.resolver.LookupCNAME(ctx, customDomain.Hostname)
	if err != nil {
		errMsg := fmt.Sprintf("DNS CNAME lookup failed: %v", err)
		_, _ = s.repo.UpdateRoutingStatus(
			ctx,
			customDomain.ID,
			customDomain.ProjectID,
			domain.CustomDomainRoutingStatusFailed,
			expectedTarget,
			&errMsg,
		)
		return nil, fmt.Errorf("%w: %v", ErrCNAMERecordNotFound, err)
	}

	actualTarget := normalizeCNAME(resolvedCNAME)

	if actualTarget != expectedTarget {
		errMsg := fmt.Sprintf("expected %s, got %s", expectedTarget, actualTarget)
		_, _ = s.repo.UpdateRoutingStatus(
			ctx,
			customDomain.ID,
			customDomain.ProjectID,
			domain.CustomDomainRoutingStatusFailed,
			expectedTarget,
			&errMsg,
		)
		return nil, ErrCNAMERoutingMismatch
	}

	updatedDomain, err := s.repo.UpdateRoutingStatus(
		ctx,
		customDomain.ID,
		customDomain.ProjectID,
		domain.CustomDomainRoutingStatusReady,
		expectedTarget,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("update routing status: %w", err)
	}

	return updatedDomain, nil
}

func normalizeCNAME(cname string) string {
	cname = strings.ToLower(strings.TrimSpace(cname))
	cname = strings.TrimSuffix(cname, ".")
	return cname
}

// ActivateCustomDomain initiates or returns the state of asynchronous custom domain provisioning.
func (s *CustomDomainService) ActivateCustomDomain(
	ctx context.Context,
	projectID uuid.UUID,
	customDomainID uuid.UUID,
) (*ActivateCustomDomainResult, error) {
	if !s.automationEnabled {
		return nil, ErrAutomationDisabled
	}

	customDomain, err := s.repo.GetByIDForProject(ctx, customDomainID, projectID)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}
		return nil, fmt.Errorf("load custom domain for activation: %w", err)
	}

	if customDomain.Status == domain.CustomDomainStatusActive || customDomain.ProvisioningStatus == domain.ProvisioningStatusCompleted {
		return &ActivateCustomDomainResult{
			Domain: customDomain,
			State:  domain.ActivationAlreadyActive,
		}, nil
	}

	switch customDomain.ProvisioningStatus {
	case domain.ProvisioningStatusRequestingCertificate,
		domain.ProvisioningStatusWaitingForValidationRecord,
		domain.ProvisioningStatusWaitingForDNS,
		domain.ProvisioningStatusWaitingForCertificate,
		domain.ProvisioningStatusAttachingCertificate:
		return &ActivateCustomDomainResult{
			Domain: customDomain,
			State:  domain.ActivationInProgress,
		}, nil
	}

	if customDomain.Status != domain.CustomDomainStatusVerified {
		return nil, ErrCustomDomainNotVerified
	}

	if customDomain.RoutingStatus != domain.CustomDomainRoutingStatusReady {
		return nil, ErrCustomDomainRoutingNotReady
	}

	if customDomain.ProvisioningStatus == domain.ProvisioningStatusFailed {
		retried, err := s.performRetry(ctx, customDomain)
		if err != nil {
			return nil, err
		}
		return &ActivateCustomDomainResult{
			Domain: retried,
			State:  domain.ActivationQueued,
		}, nil
	}

	enqueued, err := s.repo.EnqueueProvisioning(ctx, customDomainID, projectID)
	if err != nil {
		return nil, fmt.Errorf("enqueue provisioning: %w", err)
	}

	s.logger.Info().
		Str("custom_domain_id", enqueued.ID.String()).
		Str("project_id", enqueued.ProjectID.String()).
		Str("hostname", enqueued.Hostname).
		Msg("custom domain provisioning enqueued")

	return &ActivateCustomDomainResult{
		Domain: enqueued,
		State:  domain.ActivationQueued,
	}, nil
}

// GetProvisioningStatus retrieves safe, customer-facing provisioning status details for a domain.
func (s *CustomDomainService) GetProvisioningStatus(
	ctx context.Context,
	projectID uuid.UUID,
	customDomainID uuid.UUID,
) (*domain.ProvisioningStatusResponse, error) {
	cd, err := s.repo.GetByIDForProject(ctx, customDomainID, projectID)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}
		return nil, fmt.Errorf("get custom domain provisioning status: %w", err)
	}

	var sanitizedErr *string
	if cd.ProvisioningError != nil && *cd.ProvisioningError != "" {
		msg := sanitizeProvisioningError(*cd.ProvisioningError)
		sanitizedErr = &msg
	}

	var valName *string
	var valValue *string
	if cd.ProvisioningStatus == domain.ProvisioningStatusWaitingForValidationRecord ||
		cd.ProvisioningStatus == domain.ProvisioningStatusWaitingForDNS {
		valName = cd.CertificateValidationName
		valValue = cd.CertificateValidationValue
	}

	hostRoutingActive := cd.ListenerRuleARN != nil && strings.TrimSpace(*cd.ListenerRuleARN) != ""

	var gatewayType *string
	var gatewayExternalID *string

	if hostRoutingActive {
		gType := "dedicated"
		gatewayType = &gType

		gwIngress, err := s.repo.GetActiveProjectGatewayIngress(ctx, projectID)
		if err == nil && gwIngress != nil && strings.TrimSpace(gwIngress.ExternalID) != "" {
			extID := strings.TrimSpace(gwIngress.ExternalID)
			gatewayExternalID = &extID
		}
	}

	return &domain.ProvisioningStatusResponse{
		ID:                         cd.ID,
		Hostname:                   cd.Hostname,
		Status:                     cd.Status,
		RoutingStatus:              cd.RoutingStatus,
		ProvisioningStatus:         cd.ProvisioningStatus,
		CertificateStatus:          cd.CertificateStatus,
		CertificateValidationName:  valName,
		CertificateValidationValue: valValue,
		LastError:                  sanitizedErr,
		Attempts:                   cd.ProvisioningAttempts,
		NextRetryAt:                cd.NextRetryAt,
		CertificateIssuedAt:        cd.CertificateIssuedAt,
		CertificateAttachedAt:      cd.CertificateAttachedAt,
		ActivatedAt:                cd.ActivatedAt,
		GatewayExternalID:          gatewayExternalID,
		GatewayType:                gatewayType,
		HostRoutingActive:          hostRoutingActive,
	}, nil
}

// RetryProvisioning safely restarts custom domain provisioning after a terminal or transient failure.
func (s *CustomDomainService) RetryProvisioning(
	ctx context.Context,
	projectID uuid.UUID,
	customDomainID uuid.UUID,
) (*domain.CustomDomain, error) {
	if !s.automationEnabled {
		return nil, ErrAutomationDisabled
	}

	cd, err := s.repo.GetByIDForProject(ctx, customDomainID, projectID)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}
		return nil, fmt.Errorf("get custom domain for retry: %w", err)
	}

	if cd.ProvisioningStatus != domain.ProvisioningStatusFailed {
		return nil, ErrDomainNotEligibleForRetry
	}

	if cd.Status != domain.CustomDomainStatusVerified {
		return nil, ErrCustomDomainNotVerified
	}

	if cd.RoutingStatus != domain.CustomDomainRoutingStatusReady {
		return nil, ErrCustomDomainRoutingNotReady
	}

	return s.performRetry(ctx, cd)
}

func (s *CustomDomainService) performRetry(
	ctx context.Context,
	cd *domain.CustomDomain,
) (*domain.CustomDomain, error) {
	targetState := domain.ProvisioningStatusRequestingCertificate
	if cd.CertificateARN != nil && *cd.CertificateARN != "" {
		if cd.CertificateValidationName == nil || *cd.CertificateValidationName == "" {
			targetState = domain.ProvisioningStatusWaitingForValidationRecord
		} else if cd.CertificateStatus != nil && *cd.CertificateStatus == domain.CertificateStatusIssued {
			targetState = domain.ProvisioningStatusAttachingCertificate
		} else {
			targetState = domain.ProvisioningStatusWaitingForDNS
		}
	}

	return s.repo.ResetProvisioningForRetry(ctx, cd.ID, cd.ProjectID, targetState)
}

func sanitizeProvisioningError(errStr string) string {
	switch strings.ToUpper(strings.TrimSpace(errStr)) {
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
		return "An error occurred during certificate provisioning. Please retry or contact support."
	}
}
