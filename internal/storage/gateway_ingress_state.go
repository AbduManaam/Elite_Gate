package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var ErrStaleGatewayIngressLease = errors.New("gateway ingress job lease is stale")

// MarkGatewayTargetGroupCreated saves the target group ARN and moves the job
// to EC2 target registration.
func (r *GatewayRepo) MarkGatewayTargetGroupCreated(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	targetGroupARN string,
) error {
	const query = `
UPDATE gateways
SET target_group_arn     = $3,
    provisioning_status = 'registering_target',
    provisioning_error  = NULL,
    next_retry_at        = NOW(),
    locked_at            = NULL,
    locked_by            = NULL,
    lease_token          = NULL,
    updated_at           = NOW()
WHERE external_id = $1
  AND lease_token = $2
  AND deleted_at IS NULL
`

	result, err := r.db.ExecContext(ctx, query, externalID, leaseToken, targetGroupARN)
	if err != nil {
		return fmt.Errorf("save gateway target group: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}

// MarkGatewayTargetRegistered moves the job to ALB health checking.
func (r *GatewayRepo) MarkGatewayTargetRegistered(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	nextCheckAt time.Time,
) error {
	const query = `
UPDATE gateways
SET provisioning_status = 'waiting_for_target_health',
    provisioning_error  = NULL,
    next_retry_at        = $3,
    locked_at            = NULL,
    locked_by            = NULL,
    lease_token          = NULL,
    updated_at           = NOW()
WHERE external_id = $1
  AND lease_token = $2
  AND deleted_at IS NULL
`

	result, err := r.db.ExecContext(ctx, query, externalID, leaseToken, nextCheckAt)
	if err != nil {
		return fmt.Errorf("mark gateway target registered: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}

// MarkGatewayTargetHealthy moves the job to listener-rule creation.
func (r *GatewayRepo) MarkGatewayTargetHealthy(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
) error {
	const query = `
UPDATE gateways
SET provisioning_status = 'creating_listener_rule',
    provisioning_error  = NULL,
    next_retry_at        = NOW(),
    locked_at            = NULL,
    locked_by            = NULL,
    lease_token          = NULL,
    updated_at           = NOW()
WHERE external_id = $1
  AND lease_token = $2
  AND deleted_at IS NULL
`

	result, err := r.db.ExecContext(ctx, query, externalID, leaseToken)
	if err != nil {
		return fmt.Errorf("mark gateway target healthy: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}

// RescheduleGatewayHealthCheck releases the job and checks target health later.
func (r *GatewayRepo) RescheduleGatewayHealthCheck(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	nextCheckAt time.Time,
) error {
	const query = `
UPDATE gateways
SET next_retry_at = $3,
    locked_at     = NULL,
    locked_by     = NULL,
    lease_token   = NULL,
    updated_at    = NOW()
WHERE external_id = $1
  AND lease_token = $2
  AND deleted_at IS NULL
`

	result, err := r.db.ExecContext(ctx, query, externalID, leaseToken, nextCheckAt)
	if err != nil {
		return fmt.Errorf("reschedule gateway health check: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}

// MarkGatewayIngressActive saves the public HTTPS address and marks the
// dedicated gateway ready for customer traffic.
func (r *GatewayRepo) MarkGatewayIngressActive(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	ruleARN string,
	priority int,
	hostname string,
	publicEndpoint string,
) error {
	const query = `
UPDATE gateways
SET listener_rule_arn      = $3,
    listener_rule_priority = $4,
    public_host            = $5,
    public_port            = '443',
    public_endpoint        = $6,
    status                 = 'active',
    provisioning_status    = 'completed',
    provisioning_error     = NULL,
    next_retry_at           = NULL,
    locked_at               = NULL,
    locked_by               = NULL,
    lease_token             = NULL,
    provisioned_at          = NOW(),
    updated_at              = NOW()
WHERE external_id = $1
  AND lease_token = $2
  AND deleted_at IS NULL
`

	result, err := r.db.ExecContext(
		ctx,
		query,
		externalID,
		leaseToken,
		ruleARN,
		priority,
		hostname,
		publicEndpoint,
	)
	if err != nil {
		return fmt.Errorf("mark gateway ingress active: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}

// ScheduleGatewayIngressRetry stores the error and retries the same state later.
func (r *GatewayRepo) ScheduleGatewayIngressRetry(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	message string,
	nextRetryAt time.Time,
) error {
	const query = `
UPDATE gateways
SET retry_count        = retry_count + 1,
    provisioning_error = $3,
    next_retry_at      = $4,
    locked_at          = NULL,
    locked_by          = NULL,
    lease_token        = NULL,
    updated_at         = NOW()
WHERE external_id = $1
  AND lease_token = $2
  AND deleted_at IS NULL
`

	result, err := r.db.ExecContext(
		ctx,
		query,
		externalID,
		leaseToken,
		message,
		nextRetryAt,
	)
	if err != nil {
		return fmt.Errorf("schedule gateway ingress retry: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}

// MarkGatewayIngressFailed stops automatic retries after the maximum attempts.
func (r *GatewayRepo) MarkGatewayIngressFailed(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	message string,
) error {
	const query = `
UPDATE gateways
SET status              = 'failed',
    provisioning_status = 'failed',
    provisioning_error  = $3,
    next_retry_at        = NULL,
    locked_at            = NULL,
    locked_by            = NULL,
    lease_token          = NULL,
    updated_at           = NOW()
WHERE external_id = $1
  AND lease_token = $2
  AND deleted_at IS NULL
`

	result, err := r.db.ExecContext(ctx, query, externalID, leaseToken, message)
	if err != nil {
		return fmt.Errorf("mark gateway ingress failed: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}

func ensureGatewayIngressUpdated(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read gateway update result: %w", err)
	}
	if affected == 0 {
		return ErrStaleGatewayIngressLease
	}

	return nil
}
