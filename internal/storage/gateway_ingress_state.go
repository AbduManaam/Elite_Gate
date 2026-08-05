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

// After registration, the listener rule must be created before ALB health
// checks can move the target from "unused" to "healthy".
func (r *GatewayRepo) MarkGatewayTargetRegistered(
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
		return fmt.Errorf("mark gateway target registered: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}

// ReserveGatewayListenerRulePriority safely reserves one ALB listener priority.
// The database lock prevents two Workers from selecting the same priority.
func (r *GatewayRepo) ReserveGatewayListenerRulePriority(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	minPriority int,
	maxPriority int,
) (int, error) {
	if minPriority < 1 || maxPriority > 50000 || minPriority > maxPriority {
		return 0, errors.New("invalid ALB listener priority range")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin listener priority transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Only one Worker can allocate a priority at a time.
	if _, err := tx.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock(746173821)`,
	); err != nil {
		return 0, fmt.Errorf("lock listener priority allocator: %w", err)
	}

	var existingPriority int
	err = tx.QueryRowContext(
		ctx,
		`
SELECT COALESCE(listener_rule_priority, 0)
FROM gateways
WHERE external_id = $1
  AND lease_token = $2
  AND deleted_at IS NULL
FOR UPDATE
`,
		externalID,
		leaseToken,
	).Scan(&existingPriority)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrStaleGatewayIngressLease
	}
	if err != nil {
		return 0, fmt.Errorf("read existing listener priority: %w", err)
	}

	if existingPriority > 0 {
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("commit existing listener priority: %w", err)
		}
		return existingPriority, nil
	}

	var priority int
	err = tx.QueryRowContext(
		ctx,
		`
SELECT candidate
FROM generate_series($1::integer, $2::integer) AS candidate
WHERE NOT EXISTS (
SELECT 1
FROM gateways
WHERE listener_rule_priority = candidate
  AND deleted_at IS NULL
)
ORDER BY candidate
LIMIT 1
`,
		minPriority,
		maxPriority,
	).Scan(&priority)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("no ALB listener-rule priority is available")
	}
	if err != nil {
		return 0, fmt.Errorf("allocate listener priority: %w", err)
	}

	result, err := tx.ExecContext(
		ctx,
		`
UPDATE gateways
SET listener_rule_priority = $3,
    updated_at = NOW()
WHERE external_id = $1
  AND lease_token = $2
  AND deleted_at IS NULL
`,
		externalID,
		leaseToken,
		priority,
	)
	if err != nil {
		return 0, fmt.Errorf("reserve listener priority: %w", err)
	}
	if err := ensureGatewayIngressUpdated(result); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit listener priority: %w", err)
	}

	return priority, nil
}

// Save the rule and then begin checking ALB target health.
func (r *GatewayRepo) MarkGatewayListenerRuleCreated(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	ruleARN string,
	nextCheckAt time.Time,
) error {
	const query = `
UPDATE gateways
SET listener_rule_arn    = $3,
    provisioning_status = 'waiting_for_target_health',
    provisioning_error  = NULL,
    next_retry_at        = $4,
    locked_at            = NULL,
    locked_by            = NULL,
    lease_token          = NULL,
    updated_at           = NOW()
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
		nextCheckAt,
	)
	if err != nil {
		return fmt.Errorf("save gateway listener rule: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}

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

func (r *GatewayRepo) MarkGatewayIngressActive(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	hostname string,
	publicEndpoint string,
) error {
	const query = `
UPDATE gateways
SET public_host          = $3,
    public_port          = '443',
    public_endpoint      = $4,
    status               = 'active',
    provisioning_status  = 'completed',
    provisioning_error   = NULL,
    next_retry_at        = NULL,
    locked_at            = NULL,
    locked_by            = NULL,
    lease_token          = NULL,
    provisioned_at       = NOW(),
    updated_at           = NOW()
WHERE external_id = $1
  AND lease_token = $2
  AND deleted_at IS NULL
`

	result, err := r.db.ExecContext(
		ctx,
		query,
		externalID,
		leaseToken,
		hostname,
		publicEndpoint,
	)
	if err != nil {
		return fmt.Errorf("mark gateway ingress active: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}

func (r *GatewayRepo) ScheduleGatewayIngressRetry(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	message string,
	nextRetryAt time.Time,
) error {
	const query = `
UPDATE gateways
SET retry_count         = retry_count + 1,
    provisioning_error = $3,
    next_retry_at       = $4,
    locked_at           = NULL,
    locked_by           = NULL,
    lease_token         = NULL,
    updated_at          = NOW()
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

func (r *GatewayRepo) MarkGatewayIngressFailed(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
	message string,
) error {
	const query = `
UPDATE gateways
SET status               = 'failed',
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
