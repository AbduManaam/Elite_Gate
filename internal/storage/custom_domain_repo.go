package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"elitegate/internal/domain"
	"elitegate/internal/model"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

var (
	ErrCustomDomainNotFound = errors.New("custom domain not found")
	ErrHostnameConflict     = errors.New("hostname already registered")
)

// CustomDomainRepo manages custom domain persistence.
type CustomDomainRepo struct {
	BaseRepo
}

// NewCustomDomainRepo creates a CustomDomainRepo.
func NewCustomDomainRepo(
	db *sql.DB,
	logger zerolog.Logger,
) *CustomDomainRepo {
	return &CustomDomainRepo{
		BaseRepo{
			db:     db,
			logger: logger,
		},
	}
}

// HostnameExists checks whether an active custom-domain record already uses
// the supplied hostname.
func (r *CustomDomainRepo) HostnameExists(
	ctx context.Context,
	hostname string,
) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM custom_domains
			WHERE LOWER(hostname) = LOWER($1)
			  AND deleted_at IS NULL
		)
	`

	var exists bool

	if err := r.db.QueryRowContext(ctx, query, hostname).Scan(&exists); err != nil {
		return false, fmt.Errorf("check custom domain hostname: %w", err)
	}

	return exists, nil
}

// Create inserts a new custom domain.
func (r *CustomDomainRepo) Create(
	ctx context.Context,
	customDomain *domain.CustomDomain,
) error {
	const query = `
		INSERT INTO custom_domains (
			id,
			project_id,
			hostname,
			status,
			verification_token_hash,
			verification_record_name,
			certificate_arn,
			certificate_status,
			failure_reason,
			verified_at,
			activated_at,
			last_checked_at,
			routing_target,
			routing_status
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14
		)
		RETURNING created_at, updated_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		customDomain.ID,
		customDomain.ProjectID,
		customDomain.Hostname,
		customDomain.Status,
		customDomain.VerificationTokenHash,
		customDomain.VerificationRecordName,
		customDomain.CertificateARN,
		customDomain.CertificateStatus,
		customDomain.FailureReason,
		customDomain.VerifiedAt,
		customDomain.ActivatedAt,
		customDomain.LastCheckedAt,
		customDomain.RoutingTarget,
		customDomain.RoutingStatus,
	).Scan(
		&customDomain.CreatedAt,
		&customDomain.UpdatedAt,
	)

	if err != nil {
		var pqErr *pq.Error

		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return ErrHostnameConflict
		}

		return fmt.Errorf("insert custom domain: %w", err)
	}

	r.logger.Info().
		Str("custom_domain_id", customDomain.ID.String()).
		Str("project_id", customDomain.ProjectID.String()).
		Str("hostname", customDomain.Hostname).
		Msg("custom domain created")

	return nil
}

// GetByID returns one non-deleted custom domain.
func (r *CustomDomainRepo) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*domain.CustomDomain, error) {
	const query = `
		SELECT
			id,
			project_id,
			hostname,
			status,
			verification_token_hash,
			verification_record_name,
			certificate_arn,
			certificate_status,
			failure_reason,
			verified_at,
			activated_at,
			last_checked_at,
			created_at,
			updated_at,
			deleted_at,
			routing_target,
			routing_status,
			routing_checked_at,
			routing_error
		FROM custom_domains
		WHERE id = $1
		  AND deleted_at IS NULL
	`

	var customDomain domain.CustomDomain

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&customDomain.ID,
		&customDomain.ProjectID,
		&customDomain.Hostname,
		&customDomain.Status,
		&customDomain.VerificationTokenHash,
		&customDomain.VerificationRecordName,
		&customDomain.CertificateARN,
		&customDomain.CertificateStatus,
		&customDomain.FailureReason,
		&customDomain.VerifiedAt,
		&customDomain.ActivatedAt,
		&customDomain.LastCheckedAt,
		&customDomain.CreatedAt,
		&customDomain.UpdatedAt,
		&customDomain.DeletedAt,
		&customDomain.RoutingTarget,
		&customDomain.RoutingStatus,
		&customDomain.RoutingCheckedAt,
		&customDomain.RoutingError,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomDomainNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("get custom domain by id: %w", err)
	}

	return &customDomain, nil
}

// ListByProject returns all non-deleted custom domains belonging to a project.
func (r *CustomDomainRepo) ListByProject(
	ctx context.Context,
	projectID uuid.UUID,
) ([]domain.CustomDomain, error) {
	const query = `
		SELECT
			id,
			project_id,
			hostname,
			status,
			verification_token_hash,
			verification_record_name,
			certificate_arn,
			certificate_status,
			failure_reason,
			verified_at,
			activated_at,
			last_checked_at,
			created_at,
			updated_at,
			deleted_at,
			routing_target,
			routing_status,
			routing_checked_at,
			routing_error
		FROM custom_domains
		WHERE project_id = $1
		  AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list custom domains by project: %w", err)
	}
	defer rows.Close()

	customDomains := make([]domain.CustomDomain, 0)

	for rows.Next() {
		var customDomain domain.CustomDomain

		if err := rows.Scan(
			&customDomain.ID,
			&customDomain.ProjectID,
			&customDomain.Hostname,
			&customDomain.Status,
			&customDomain.VerificationTokenHash,
			&customDomain.VerificationRecordName,
			&customDomain.CertificateARN,
			&customDomain.CertificateStatus,
			&customDomain.FailureReason,
			&customDomain.VerifiedAt,
			&customDomain.ActivatedAt,
			&customDomain.LastCheckedAt,
			&customDomain.CreatedAt,
			&customDomain.UpdatedAt,
			&customDomain.DeletedAt,
			&customDomain.RoutingTarget,
			&customDomain.RoutingStatus,
			&customDomain.RoutingCheckedAt,
			&customDomain.RoutingError,
		); err != nil {
			return nil, fmt.Errorf("scan custom domain row: %w", err)
		}

		customDomains = append(customDomains, customDomain)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom domain rows: %w", err)
	}

	return customDomains, nil
}

// GetByIDForProject returns a non-deleted custom domain belonging to a
// particular project.
func (r *CustomDomainRepo) GetByIDForProject(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
) (*domain.CustomDomain, error) {
	const query = `
		SELECT
			id,
			project_id,
			hostname,
			status,
			verification_token_hash,
			verification_record_name,
			certificate_arn,
			certificate_status,
			failure_reason,
			verified_at,
			activated_at,
			last_checked_at,
			created_at,
			updated_at,
			deleted_at,
			routing_target,
			routing_status,
			routing_checked_at,
			routing_error
		FROM custom_domains
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
	`

	var customDomain domain.CustomDomain

	err := r.db.QueryRowContext(
		ctx,
		query,
		id,
		projectID,
	).Scan(
		&customDomain.ID,
		&customDomain.ProjectID,
		&customDomain.Hostname,
		&customDomain.Status,
		&customDomain.VerificationTokenHash,
		&customDomain.VerificationRecordName,
		&customDomain.CertificateARN,
		&customDomain.CertificateStatus,
		&customDomain.FailureReason,
		&customDomain.VerifiedAt,
		&customDomain.ActivatedAt,
		&customDomain.LastCheckedAt,
		&customDomain.CreatedAt,
		&customDomain.UpdatedAt,
		&customDomain.DeletedAt,
		&customDomain.RoutingTarget,
		&customDomain.RoutingStatus,
		&customDomain.RoutingCheckedAt,
		&customDomain.RoutingError,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomDomainNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get custom domain by id and project: %w",
			err,
		)
	}

	return &customDomain, nil
}

// MarkVerified updates a custom domain after successful TXT verification.
func (r *CustomDomainRepo) MarkVerified(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
) (*domain.CustomDomain, error) {
	const query = `
		UPDATE custom_domains
		SET
			status = $3,
			verified_at = NOW(),
			last_checked_at = NOW(),
			failure_reason = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
		RETURNING
			id,
			project_id,
			hostname,
			status,
			verification_token_hash,
			verification_record_name,
			certificate_arn,
			certificate_status,
			failure_reason,
			verified_at,
			activated_at,
			last_checked_at,
			created_at,
			updated_at,
			deleted_at,
			routing_target,
			routing_status,
			routing_checked_at,
			routing_error
	`

	var customDomain domain.CustomDomain

	err := r.db.QueryRowContext(
		ctx,
		query,
		id,
		projectID,
		domain.CustomDomainStatusVerified,
	).Scan(
		&customDomain.ID,
		&customDomain.ProjectID,
		&customDomain.Hostname,
		&customDomain.Status,
		&customDomain.VerificationTokenHash,
		&customDomain.VerificationRecordName,
		&customDomain.CertificateARN,
		&customDomain.CertificateStatus,
		&customDomain.FailureReason,
		&customDomain.VerifiedAt,
		&customDomain.ActivatedAt,
		&customDomain.LastCheckedAt,
		&customDomain.CreatedAt,
		&customDomain.UpdatedAt,
		&customDomain.DeletedAt,
		&customDomain.RoutingTarget,
		&customDomain.RoutingStatus,
		&customDomain.RoutingCheckedAt,
		&customDomain.RoutingError,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomDomainNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("mark custom domain verified: %w", err)
	}

	return &customDomain, nil
}

// RecordVerificationFailure records a domain verification failure by updating
// last_checked_at and failure_reason.
func (r *CustomDomainRepo) RecordVerificationFailure(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
	reason string,
) error {
	const query = `
		UPDATE custom_domains
		SET
			last_checked_at = NOW(),
			failure_reason = $3,
			updated_at = NOW()
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		id,
		projectID,
		reason,
	)
	if err != nil {
		return fmt.Errorf(
			"record custom-domain verification failure: %w",
			err,
		)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf(
			"read verification failure update count: %w",
			err,
		)
	}

	if rowsAffected == 0 {
		return ErrCustomDomainNotFound
	}

	return nil
}

// SoftDelete soft deletes a custom domain by setting deleted_at to NOW().
func (r *CustomDomainRepo) SoftDelete(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
) error {
	const query = `
		UPDATE custom_domains
		SET
			deleted_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, projectID)
	if err != nil {
		return fmt.Errorf("soft delete custom domain: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read soft delete rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrCustomDomainNotFound
	}

	return nil
}

// UpdateRoutingStatus updates the CNAME routing status, target, checked_at, and error for a custom domain.
func (r *CustomDomainRepo) UpdateRoutingStatus(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
	status string,
	target string,
	routingError *string,
) (*domain.CustomDomain, error) {
	const query = `
		UPDATE custom_domains
		SET
			routing_status = $3,
			routing_target = $4,
			routing_checked_at = NOW(),
			routing_error = $5,
			updated_at = NOW()
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
		RETURNING
			id,
			project_id,
			hostname,
			status,
			verification_token_hash,
			verification_record_name,
			certificate_arn,
			certificate_status,
			failure_reason,
			verified_at,
			activated_at,
			last_checked_at,
			created_at,
			updated_at,
			deleted_at,
			routing_target,
			routing_status,
			routing_checked_at,
			routing_error
	`

	var customDomain domain.CustomDomain

	err := r.db.QueryRowContext(
		ctx,
		query,
		id,
		projectID,
		status,
		target,
		routingError,
	).Scan(
		&customDomain.ID,
		&customDomain.ProjectID,
		&customDomain.Hostname,
		&customDomain.Status,
		&customDomain.VerificationTokenHash,
		&customDomain.VerificationRecordName,
		&customDomain.CertificateARN,
		&customDomain.CertificateStatus,
		&customDomain.FailureReason,
		&customDomain.VerifiedAt,
		&customDomain.ActivatedAt,
		&customDomain.LastCheckedAt,
		&customDomain.CreatedAt,
		&customDomain.UpdatedAt,
		&customDomain.DeletedAt,
		&customDomain.RoutingTarget,
		&customDomain.RoutingStatus,
		&customDomain.RoutingCheckedAt,
		&customDomain.RoutingError,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomDomainNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("update custom domain routing status: %w", err)
	}

	return &customDomain, nil
}

// ListEligibleSyncDomains returns all non-deleted custom domains for a project
// that are verified and routing-ready, ordered stably by creation timestamp.
func (r *CustomDomainRepo) ListEligibleSyncDomains(
	ctx context.Context,
	projectID uuid.UUID,
) ([]model.CustomDomainSync, error) {
	const query = `
		SELECT
			hostname,
			status,
			routing_status
		FROM custom_domains
		WHERE project_id = $1
		  AND deleted_at IS NULL
		  AND status = 'verified'
		  AND routing_status = 'ready'
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list eligible custom domains for sync: %w", err)
	}
	defer rows.Close()

	domains := make([]model.CustomDomainSync, 0)
	for rows.Next() {
		var d model.CustomDomainSync
		if err := rows.Scan(&d.Hostname, &d.Status, &d.RoutingStatus); err != nil {
			return nil, fmt.Errorf("scan eligible custom domain sync row: %w", err)
		}
		domains = append(domains, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate eligible custom domain sync rows: %w", err)
	}

	return domains, nil
}
