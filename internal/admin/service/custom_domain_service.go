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
}

// DNSResolver represents a DNS resolver capable of querying TXT and CNAME records.
//
// Using an interface allows the DNS behavior to be mocked in unit tests.
type DNSResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
}

// CustomDomainService handles custom-domain business logic.
type CustomDomainService struct {
	repo              CustomDomainRepository
	resolver          DNSResolver
	gatewayPublicHost string
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

// DeleteCustomDomain soft deletes a custom domain for a project.
func (s *CustomDomainService) DeleteCustomDomain(
	ctx context.Context,
	projectID uuid.UUID,
	customDomainID uuid.UUID,
) error {
	err := s.repo.SoftDelete(
		ctx,
		customDomainID,
		projectID,
	)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return ErrCustomDomainNotFound
		}

		return fmt.Errorf("delete custom domain: %w", err)
	}

	s.logger.Info().
		Str("custom_domain_id", customDomainID.String()).
		Str("project_id", projectID.String()).
		Msg("custom domain soft deleted")

	return nil
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

// ActivateCustomDomain promotes a verified and routing-ready domain to active status.
// If the domain is already active and routing is ready, it idempotently returns the existing domain.
func (s *CustomDomainService) ActivateCustomDomain(
	ctx context.Context,
	projectID uuid.UUID,
	customDomainID uuid.UUID,
) (*domain.CustomDomain, error) {
	customDomain, err := s.repo.GetByIDForProject(ctx, customDomainID, projectID)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}
		return nil, fmt.Errorf("load custom domain for activation: %w", err)
	}

	if customDomain.Status == domain.CustomDomainStatusActive {
		if customDomain.RoutingStatus == domain.CustomDomainRoutingStatusReady {
			return customDomain, nil
		}
		return nil, ErrCustomDomainRoutingNotReady
	}

	if customDomain.Status != domain.CustomDomainStatusVerified {
		return nil, ErrCustomDomainNotVerified
	}

	if customDomain.RoutingStatus != domain.CustomDomainRoutingStatusReady {
		return nil, ErrCustomDomainRoutingNotReady
	}

	activatedDomain, err := s.repo.MarkActive(ctx, customDomainID, projectID)
	if err != nil {
		if errors.Is(err, storage.ErrCustomDomainNotFound) {
			return nil, ErrCustomDomainNotFound
		}
		return nil, fmt.Errorf("mark custom domain active: %w", err)
	}

	s.logger.Info().
		Str("custom_domain_id", activatedDomain.ID.String()).
		Str("project_id", activatedDomain.ProjectID.String()).
		Str("hostname", activatedDomain.Hostname).
		Msg("custom domain activated")

	return activatedDomain, nil
}
