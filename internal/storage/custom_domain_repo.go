package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"elitegate/internal/domain"
	"elitegate/internal/model"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

var (
	ErrCustomDomainNotFound          = errors.New("custom domain not found")
	ErrHostnameConflict              = errors.New("hostname already registered")
	ErrStaleLease                    = errors.New("stale lease token or lock expired")
	ErrInvalidStateTransition        = errors.New("invalid provisioning state transition")
	ErrDomainNotEligible             = errors.New("custom domain not eligible for provisioning")
	ErrDomainNotEligibleForRetry     = errors.New("custom domain not eligible for retry")
	ErrDomainAlreadyDeprovisioned    = errors.New("custom domain already deprovisioned")
	ErrProjectGatewayIngressNotReady = errors.New("project dedicated gateway ingress is not ready")
)

type ProjectGatewayIngress struct {
	ExternalID     string
	TargetGroupARN string
}

const customDomainAllColumns = `
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
	routing_error,
	certificate_managed_by_elitegate,
	provisioning_status,
	certificate_validation_name,
	certificate_validation_value,
	certificate_requested_at,
	certificate_issued_at,
	certificate_attached_at,
	provisioning_started_at,
	provisioning_completed_at,
	deprovisioning_started_at,
	provisioning_error,
	provisioning_attempts,
	next_retry_at,
	listener_rule_arn,
	listener_rule_priority,
	locked_at,
	locked_by,
	lease_token
`

func scanCustomDomainRow(scanner interface{ Scan(dest ...any) error }) (*domain.CustomDomain, error) {
	var cd domain.CustomDomain
	err := scanner.Scan(
		&cd.ID,
		&cd.ProjectID,
		&cd.Hostname,
		&cd.Status,
		&cd.VerificationTokenHash,
		&cd.VerificationRecordName,
		&cd.CertificateARN,
		&cd.CertificateStatus,
		&cd.FailureReason,
		&cd.VerifiedAt,
		&cd.ActivatedAt,
		&cd.LastCheckedAt,
		&cd.CreatedAt,
		&cd.UpdatedAt,
		&cd.DeletedAt,
		&cd.RoutingTarget,
		&cd.RoutingStatus,
		&cd.RoutingCheckedAt,
		&cd.RoutingError,
		&cd.CertificateManagedByEliteGate,
		&cd.ProvisioningStatus,
		&cd.CertificateValidationName,
		&cd.CertificateValidationValue,
		&cd.CertificateRequestedAt,
		&cd.CertificateIssuedAt,
		&cd.CertificateAttachedAt,
		&cd.ProvisioningStartedAt,
		&cd.ProvisioningCompletedAt,
		&cd.DeprovisioningStartedAt,
		&cd.ProvisioningError,
		&cd.ProvisioningAttempts,
		&cd.NextRetryAt,
		&cd.ListenerRuleARN,
		&cd.ListenerRulePriority,
		&cd.LockedAt,
		&cd.LockedBy,
		&cd.LeaseToken,
	)
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

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
	if customDomain.ProvisioningStatus == "" {
		customDomain.ProvisioningStatus = domain.ProvisioningStatusNotStarted
	}
	if customDomain.CertificateStatus == nil {
		notRequested := "not_requested"
		customDomain.CertificateStatus = &notRequested
	}

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
			routing_status,
			certificate_managed_by_elitegate,
			provisioning_status
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16
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
		customDomain.CertificateManagedByEliteGate,
		customDomain.ProvisioningStatus,
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
	query := fmt.Sprintf(`
		SELECT %s
		FROM custom_domains
		WHERE id = $1
		  AND deleted_at IS NULL
	`, customDomainAllColumns)

	row := r.db.QueryRowContext(ctx, query, id)
	cd, err := scanCustomDomainRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get custom domain by id: %w", err)
	}

	return cd, nil
}

// ListByProject returns all non-deleted custom domains belonging to a project.
func (r *CustomDomainRepo) ListByProject(
	ctx context.Context,
	projectID uuid.UUID,
) ([]domain.CustomDomain, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM custom_domains
		WHERE project_id = $1
		  AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, customDomainAllColumns)

	rows, err := r.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list custom domains by project: %w", err)
	}
	defer rows.Close()

	customDomains := make([]domain.CustomDomain, 0)

	for rows.Next() {
		cd, err := scanCustomDomainRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan custom domain row: %w", err)
		}
		customDomains = append(customDomains, *cd)
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
	query := fmt.Sprintf(`
		SELECT %s
		FROM custom_domains
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
	`, customDomainAllColumns)

	row := r.db.QueryRowContext(ctx, query, id, projectID)
	cd, err := scanCustomDomainRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf(
			"get custom domain by id and project: %w",
			err,
		)
	}

	return cd, nil
}

// MarkVerified updates a custom domain after successful TXT verification.
func (r *CustomDomainRepo) MarkVerified(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
) (*domain.CustomDomain, error) {
	query := fmt.Sprintf(`
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
		RETURNING %s
	`, customDomainAllColumns)

	row := r.db.QueryRowContext(
		ctx,
		query,
		id,
		projectID,
		domain.CustomDomainStatusVerified,
	)
	cd, err := scanCustomDomainRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mark custom domain verified: %w", err)
	}

	return cd, nil
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
	query := fmt.Sprintf(`
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
		RETURNING %s
	`, customDomainAllColumns)

	row := r.db.QueryRowContext(
		ctx,
		query,
		id,
		projectID,
		status,
		target,
		routingError,
	)
	cd, err := scanCustomDomainRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update custom domain routing status: %w", err)
	}

	return cd, nil
}

// MarkActive updates a custom domain status to active if verified and routing-ready.
func (r *CustomDomainRepo) MarkActive(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
) (*domain.CustomDomain, error) {
	query := fmt.Sprintf(`
		UPDATE custom_domains
		SET
			status = $3,
			activated_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
		  AND status = 'verified'
		  AND routing_status = 'ready'
		RETURNING %s
	`, customDomainAllColumns)

	row := r.db.QueryRowContext(
		ctx,
		query,
		id,
		projectID,
		domain.CustomDomainStatusActive,
	)
	cd, err := scanCustomDomainRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mark custom domain active: %w", err)
	}

	return cd, nil
}

// ListEligibleSyncDomains returns all non-deleted custom domains for a project
// that are active and routing-ready, ordered stably by creation timestamp.
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
		  AND status = 'active'
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

// AdvanceProvisioningParams contains parameters for advancing provisioning state.
type AdvanceProvisioningParams struct {
	ID                            uuid.UUID
	LeaseToken                    uuid.UUID
	ExpectedStatus                string
	NewStatus                     string
	CertificateARN                *string
	CertificateStatus             *string
	CertificateManagedByEliteGate *bool
	CertificateValidationName     *string
	CertificateValidationValue    *string
	CertificateRequestedAt        *time.Time
	CertificateIssuedAt           *time.Time
	CertificateAttachedAt         *time.Time
	NextRetryAt                   *time.Time
}

func scanProvisioningJobRow(scanner interface{ Scan(dest ...any) error }) (*domain.ProvisioningJob, error) {
	var job domain.ProvisioningJob
	err := scanner.Scan(
		&job.ID,
		&job.ProjectID,
		&job.Hostname,
		&job.Status,
		&job.RoutingStatus,
		&job.ProvisioningStatus,
		&job.CertificateARN,
		&job.CertificateStatus,
		&job.CertificateManagedByEliteGate,
		&job.CertificateValidationName,
		&job.CertificateValidationValue,
		&job.ListenerRuleARN,
		&job.ListenerRulePriority,
		&job.ProvisioningAttempts,
		&job.NextRetryAt,
		&job.ProvisioningStartedAt,
		&job.LockedAt,
		&job.LockedBy,
		&job.LeaseToken,
		&job.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *CustomDomainRepo) checkStaleOrTransitionError(
	ctx context.Context,
	id uuid.UUID,
	leaseToken uuid.UUID,
	expectedStatus string,
) error {
	const query = `
		SELECT lease_token, provisioning_status, deleted_at
		FROM custom_domains
		WHERE id = $1
	`
	var currentLeaseStr sql.NullString
	var currentStatus string
	var deletedAt *time.Time

	err := r.db.QueryRowContext(ctx, query, id).Scan(&currentLeaseStr, &currentStatus, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCustomDomainNotFound
	}
	if err != nil {
		return fmt.Errorf("check domain transition status: %w", err)
	}

	if deletedAt != nil {
		return ErrStaleLease
	}

	if !currentLeaseStr.Valid || currentLeaseStr.String != leaseToken.String() {
		return ErrStaleLease
	}

	if currentStatus != expectedStatus {
		return ErrInvalidStateTransition
	}

	return ErrStaleLease
}

// ClaimNextProvisioningJob claims the next eligible domain provisioning job using atomic locking.
func (r *CustomDomainRepo) ClaimNextProvisioningJob(
	ctx context.Context,
	workerID string,
	lockTimeout time.Duration,
) (*domain.ProvisioningJob, error) {
	if lockTimeout <= 0 {
		lockTimeout = 5 * time.Minute
	}
	lockExpiry := time.Now().Add(-lockTimeout)
	leaseToken := uuid.New()

	const query = `
		WITH next_job AS (
			SELECT id
			FROM custom_domains
			WHERE deleted_at IS NULL
			  AND provisioning_status IN (
			      'requesting_certificate',
			      'waiting_for_validation_record',
			      'waiting_for_dns',
			      'waiting_for_certificate',
			      'attaching_certificate',
			      'deprovisioning'
			  )
			  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
			  AND (locked_at IS NULL OR locked_at <= $1)
			ORDER BY next_retry_at ASC NULLS FIRST, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE custom_domains cd
		SET locked_at = NOW(),
		    locked_by = $2,
		    lease_token = $3,
		    updated_at = NOW()
		FROM next_job
		WHERE cd.id = next_job.id
		RETURNING
			cd.id,
			cd.project_id,
			cd.hostname,
			cd.status,
			cd.routing_status,
			cd.provisioning_status,
			cd.certificate_arn,
			cd.certificate_status,
			cd.certificate_managed_by_elitegate,
			cd.certificate_validation_name,
			cd.certificate_validation_value,
			cd.listener_rule_arn,
			cd.listener_rule_priority,
			cd.provisioning_attempts,
			cd.next_retry_at,
			cd.provisioning_started_at,
			cd.locked_at,
			cd.locked_by,
			cd.lease_token,
			cd.deleted_at
	`

	row := r.db.QueryRowContext(ctx, query, lockExpiry, workerID, leaseToken)
	job, err := scanProvisioningJobRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim next provisioning job: %w", err)
	}

	return job, nil
}

// AdvanceProvisioningState safely updates provisioning state and optional certificate properties.
func (r *CustomDomainRepo) AdvanceProvisioningState(
	ctx context.Context,
	params AdvanceProvisioningParams,
) error {
	const query = `
		UPDATE custom_domains
		SET
			provisioning_status = $3,
			certificate_arn = COALESCE($4, certificate_arn),
			certificate_status = COALESCE($5, certificate_status),
			certificate_managed_by_elitegate = COALESCE($6, certificate_managed_by_elitegate),
			certificate_validation_name = COALESCE($7, certificate_validation_name),
			certificate_validation_value = COALESCE($8, certificate_validation_value),
			certificate_requested_at = COALESCE($9, certificate_requested_at),
			certificate_issued_at = COALESCE($10, certificate_issued_at),
			certificate_attached_at = COALESCE($11, certificate_attached_at),
			next_retry_at = $12,
			provisioning_error = NULL,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND lease_token = $2
		  AND provisioning_status = $13
		  AND deleted_at IS NULL
	`

	res, err := r.db.ExecContext(
		ctx,
		query,
		params.ID,
		params.LeaseToken,
		params.NewStatus,
		params.CertificateARN,
		params.CertificateStatus,
		params.CertificateManagedByEliteGate,
		params.CertificateValidationName,
		params.CertificateValidationValue,
		params.CertificateRequestedAt,
		params.CertificateIssuedAt,
		params.CertificateAttachedAt,
		params.NextRetryAt,
		params.ExpectedStatus,
	)
	if err != nil {
		return fmt.Errorf("advance provisioning state: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check advance provisioning state rows affected: %w", err)
	}
	if rows == 0 {
		return r.checkStaleOrTransitionError(ctx, params.ID, params.LeaseToken, params.ExpectedStatus)
	}

	return nil
}

// ScheduleProvisioningPoll clears the lease and schedules next poll without incrementing attempts.
func (r *CustomDomainRepo) ScheduleProvisioningPoll(
	ctx context.Context,
	id uuid.UUID,
	leaseToken uuid.UUID,
	expectedStatus string,
	nextPollAt time.Time,
) error {
	const query = `
		UPDATE custom_domains
		SET
			next_retry_at = $4,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND lease_token = $2
		  AND provisioning_status = $3
		  AND deleted_at IS NULL
	`

	res, err := r.db.ExecContext(ctx, query, id, leaseToken, expectedStatus, nextPollAt)
	if err != nil {
		return fmt.Errorf("schedule provisioning poll: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check schedule provisioning poll rows affected: %w", err)
	}
	if rows == 0 {
		return r.checkStaleOrTransitionError(ctx, id, leaseToken, expectedStatus)
	}

	return nil
}

// ScheduleProvisioningRetry increments attempts, records internal error, and sets retry time.
func (r *CustomDomainRepo) ScheduleProvisioningRetry(
	ctx context.Context,
	id uuid.UUID,
	leaseToken uuid.UUID,
	expectedStatus string,
	nextRetryAt time.Time,
	provisioningError string,
) error {
	const query = `
		UPDATE custom_domains
		SET
			provisioning_attempts = provisioning_attempts + 1,
			provisioning_error = $5,
			next_retry_at = $4,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND lease_token = $2
		  AND provisioning_status = $3
		  AND deleted_at IS NULL
	`

	res, err := r.db.ExecContext(ctx, query, id, leaseToken, expectedStatus, nextRetryAt, provisioningError)
	if err != nil {
		return fmt.Errorf("schedule provisioning retry: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check schedule provisioning retry rows affected: %w", err)
	}
	if rows == 0 {
		return r.checkStaleOrTransitionError(ctx, id, leaseToken, expectedStatus)
	}

	return nil
}

// MarkProvisioningFailed records terminal provisioning failure while preserving certificate metadata.
func (r *CustomDomainRepo) MarkProvisioningFailed(
	ctx context.Context,
	id uuid.UUID,
	leaseToken uuid.UUID,
	expectedStatus string,
	provisioningError string,
) error {
	const query = `
		UPDATE custom_domains
		SET
			provisioning_status = $4,
			provisioning_error = $5,
			provisioning_attempts = provisioning_attempts + 1,
			next_retry_at = NULL,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND lease_token = $2
		  AND provisioning_status = $3
		  AND deleted_at IS NULL
	`

	res, err := r.db.ExecContext(
		ctx,
		query,
		id,
		leaseToken,
		expectedStatus,
		domain.ProvisioningStatusFailed,
		provisioningError,
	)
	if err != nil {
		return fmt.Errorf("mark provisioning failed: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check mark provisioning failed rows affected: %w", err)
	}
	if rows == 0 {
		return r.checkStaleOrTransitionError(ctx, id, leaseToken, expectedStatus)
	}

	return nil
}

// GetActiveProjectGatewayIngress retrieves the project's active dedicated gateway target group.
func (r *CustomDomainRepo) GetActiveProjectGatewayIngress(
	ctx context.Context,
	projectID uuid.UUID,
) (*ProjectGatewayIngress, error) {
	const query = `
		SELECT external_id, target_group_arn
		FROM gateways
		WHERE project_id = $1
		  AND deleted_at IS NULL
		  AND plan = 'dedicated'
		  AND status = 'active'
		  AND provisioning_status = 'completed'
		  AND target_group_arn IS NOT NULL
		  AND BTRIM(target_group_arn) <> ''
		ORDER BY provisioned_at DESC NULLS LAST, created_at DESC
		LIMIT 1;
	`

	var ingress ProjectGatewayIngress
	err := r.db.QueryRowContext(ctx, query, projectID).Scan(&ingress.ExternalID, &ingress.TargetGroupARN)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProjectGatewayIngressNotReady
	}
	if err != nil {
		return nil, fmt.Errorf("get active project gateway ingress: %w", err)
	}

	return &ingress, nil
}

// MarkProvisioningCompleted completes provisioning, sets domain status to active, saves listener rule metadata, and clears locks.
func (r *CustomDomainRepo) MarkProvisioningCompleted(
	ctx context.Context,
	id uuid.UUID,
	leaseToken uuid.UUID,
	listenerRuleARN string,
	listenerRulePriority int,
) error {
	const query = `
		UPDATE custom_domains
		SET
			status = $3,
			provisioning_status = $4,
			certificate_status = $5,
			certificate_attached_at = COALESCE(certificate_attached_at, NOW()),
			provisioning_completed_at = NOW(),
			activated_at = COALESCE(activated_at, NOW()),
			listener_rule_arn = $7,
			listener_rule_priority = $8,
			provisioning_error = NULL,
			next_retry_at = NULL,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND lease_token = $2
		  AND provisioning_status = $6
		  AND deleted_at IS NULL
	`

	res, err := r.db.ExecContext(
		ctx,
		query,
		id,
		leaseToken,
		domain.CustomDomainStatusActive,
		domain.ProvisioningStatusCompleted,
		"issued",
		domain.ProvisioningStatusAttachingCertificate,
		listenerRuleARN,
		listenerRulePriority,
	)
	if err != nil {
		return fmt.Errorf("mark provisioning completed: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check mark provisioning completed rows affected: %w", err)
	}
	if rows == 0 {
		return r.checkStaleOrTransitionError(ctx, id, leaseToken, domain.ProvisioningStatusAttachingCertificate)
	}

	return nil
}

// MarkDeprovisioned sets deleted_at timestamp and marks status as deprovisioned.
func (r *CustomDomainRepo) MarkDeprovisioned(
	ctx context.Context,
	id uuid.UUID,
	leaseToken uuid.UUID,
) error {
	const query = `
		UPDATE custom_domains
		SET
			deleted_at = NOW(),
			provisioning_status = $3,
			next_retry_at = NULL,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND lease_token = $2
		  AND provisioning_status = $4
	`

	res, err := r.db.ExecContext(
		ctx,
		query,
		id,
		leaseToken,
		domain.ProvisioningStatusDeprovisioned,
		domain.ProvisioningStatusDeprovisioning,
	)
	if err != nil {
		return fmt.Errorf("mark deprovisioned: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check mark deprovisioned rows affected: %w", err)
	}
	if rows == 0 {
		return r.checkStaleOrTransitionError(ctx, id, leaseToken, domain.ProvisioningStatusDeprovisioning)
	}

	return nil
}

// ReleaseProvisioningLease clears lock and lease fields without altering domain status or retries.
func (r *CustomDomainRepo) ReleaseProvisioningLease(
	ctx context.Context,
	id uuid.UUID,
	leaseToken uuid.UUID,
) error {
	const query = `
		UPDATE custom_domains
		SET
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND lease_token = $2
		  AND deleted_at IS NULL
	`

	res, err := r.db.ExecContext(ctx, query, id, leaseToken)
	if err != nil {
		return fmt.Errorf("release provisioning lease: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check release provisioning lease rows affected: %w", err)
	}
	if rows == 0 {
		return ErrStaleLease
	}

	return nil
}

// EnqueueProvisioning transitions verified and routing-ready domains into the provisioning queue.
func (r *CustomDomainRepo) EnqueueProvisioning(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
) (*domain.CustomDomain, error) {
	query := fmt.Sprintf(`
		UPDATE custom_domains
		SET
			provisioning_status = $3,
			provisioning_started_at = NOW(),
			provisioning_attempts = 0,
			next_retry_at = NOW(),
			provisioning_error = NULL,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
		  AND status = $4
		  AND routing_status = $5
		  AND provisioning_status IN ($6, $7)
		RETURNING %s
	`, customDomainAllColumns)

	row := r.db.QueryRowContext(
		ctx,
		query,
		id,
		projectID,
		domain.ProvisioningStatusRequestingCertificate,
		domain.CustomDomainStatusVerified,
		domain.CustomDomainRoutingStatusReady,
		domain.ProvisioningStatusNotStarted,
		domain.ProvisioningStatusFailed,
	)

	cd, err := scanCustomDomainRow(row)
	if err == nil {
		return cd, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("enqueue provisioning update: %w", err)
	}

	existing, getErr := r.GetByIDForProject(ctx, id, projectID)
	if getErr != nil {
		return nil, getErr
	}

	if existing.Status != domain.CustomDomainStatusVerified || existing.RoutingStatus != domain.CustomDomainRoutingStatusReady {
		return nil, ErrDomainNotEligible
	}

	switch existing.ProvisioningStatus {
	case domain.ProvisioningStatusRequestingCertificate,
		domain.ProvisioningStatusWaitingForValidationRecord,
		domain.ProvisioningStatusWaitingForDNS,
		domain.ProvisioningStatusWaitingForCertificate,
		domain.ProvisioningStatusAttachingCertificate,
		domain.ProvisioningStatusCompleted:
		return existing, nil
	default:
		return nil, ErrDomainNotEligible
	}
}

// ResetProvisioningForRetry resets a failed provisioning job into a specified target resume state.
func (r *CustomDomainRepo) ResetProvisioningForRetry(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
	targetStatus string,
) (*domain.CustomDomain, error) {
	query := fmt.Sprintf(`
		UPDATE custom_domains
		SET
			provisioning_status = $3,
			provisioning_started_at = NOW(),
			provisioning_attempts = 0,
			next_retry_at = NOW(),
			provisioning_error = NULL,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
		  AND provisioning_status = $4
		RETURNING %s
	`, customDomainAllColumns)

	row := r.db.QueryRowContext(
		ctx,
		query,
		id,
		projectID,
		targetStatus,
		domain.ProvisioningStatusFailed,
	)

	cd, err := scanCustomDomainRow(row)
	if err == nil {
		return cd, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDomainNotEligibleForRetry
	}

	return nil, fmt.Errorf("reset provisioning for retry update: %w", err)
}

// EnqueueDeprovisioning transitions a custom domain into the deprovisioning queue.
func (r *CustomDomainRepo) EnqueueDeprovisioning(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
) (*domain.CustomDomain, error) {
	query := fmt.Sprintf(`
		UPDATE custom_domains
		SET
			provisioning_status = $3,
			deprovisioning_started_at = NOW(),
			next_retry_at = NOW(),
			provisioning_error = NULL,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
		  AND provisioning_status NOT IN ('deprovisioning', 'deprovisioned')
		RETURNING %s
	`, customDomainAllColumns)

	row := r.db.QueryRowContext(
		ctx,
		query,
		id,
		projectID,
		domain.ProvisioningStatusDeprovisioning,
	)

	cd, err := scanCustomDomainRow(row)
	if err == nil {
		return cd, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("enqueue deprovisioning update: %w", err)
	}

	existing, getErr := r.GetByIDForProject(ctx, id, projectID)
	if getErr != nil {
		return nil, getErr
	}

	if existing.ProvisioningStatus == domain.ProvisioningStatusDeprovisioning ||
		existing.ProvisioningStatus == domain.ProvisioningStatusDeprovisioned ||
		existing.DeletedAt != nil {
		return existing, nil
	}

	return nil, ErrDomainAlreadyDeprovisioned
}

// MarkDeprovisionFailed records a terminal failure during asynchronous deprovisioning.
func (r *CustomDomainRepo) MarkDeprovisionFailed(
	ctx context.Context,
	id uuid.UUID,
	leaseToken uuid.UUID,
	errStr string,
) error {
	const query = `
		UPDATE custom_domains
		SET
			provisioning_status = $3,
			provisioning_error = $4,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND lease_token = $2
		  AND provisioning_status = $5
	`

	res, err := r.db.ExecContext(
		ctx,
		query,
		id,
		leaseToken,
		domain.ProvisioningStatusDeprovisionFailed,
		errStr,
		domain.ProvisioningStatusDeprovisioning,
	)
	if err != nil {
		return fmt.Errorf("mark deprovision failed: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check mark deprovision failed rows affected: %w", err)
	}
	if rows == 0 {
		return r.checkStaleOrTransitionError(ctx, id, leaseToken, domain.ProvisioningStatusDeprovisioning)
	}

	return nil
}

// ResetDeprovisioningForRetry resets a deprovision_failed job back into deprovisioning.
func (r *CustomDomainRepo) ResetDeprovisioningForRetry(
	ctx context.Context,
	id uuid.UUID,
	projectID uuid.UUID,
) (*domain.CustomDomain, error) {
	query := fmt.Sprintf(`
		UPDATE custom_domains
		SET
			provisioning_status = $3,
			deprovisioning_started_at = NOW(),
			provisioning_attempts = 0,
			next_retry_at = NOW(),
			provisioning_error = NULL,
			locked_at = NULL,
			locked_by = NULL,
			lease_token = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND project_id = $2
		  AND deleted_at IS NULL
		  AND provisioning_status = $4
		RETURNING %s
	`, customDomainAllColumns)

	row := r.db.QueryRowContext(
		ctx,
		query,
		id,
		projectID,
		domain.ProvisioningStatusDeprovisioning,
		domain.ProvisioningStatusDeprovisionFailed,
	)

	cd, err := scanCustomDomainRow(row)
	if err == nil {
		return cd, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDomainNotEligibleForRetry
	}

	return nil, fmt.Errorf("reset deprovisioning for retry update: %w", err)
}

// GetProvisioningQueueDepth retrieves backlog counts grouped by provisioning status.
func (r *CustomDomainRepo) GetProvisioningQueueDepth(ctx context.Context) (map[string]int64, error) {
	const query = `
		SELECT provisioning_status, COUNT(*)
		FROM custom_domains
		WHERE deleted_at IS NULL
		  AND provisioning_status IN (
		      'requesting_certificate',
		      'waiting_for_validation_record',
		      'waiting_for_dns',
		      'waiting_for_certificate',
		      'attaching_certificate',
		      'deprovisioning'
		  )
		GROUP BY provisioning_status
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query queue depth: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan queue depth row: %w", err)
		}
		counts[status] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue depth rows: %w", err)
	}

	return counts, nil
}
