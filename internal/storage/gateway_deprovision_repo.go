package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// QueueGatewayDeprovisioning moves a gateway into the asynchronous AWS cleanup
// workflow. Repeated delete requests preserve the original drain time and any
// retry already scheduled by the Worker.
func (r *GatewayRepo) QueueGatewayDeprovisioning(
	ctx context.Context,
	externalID string,
	drainTimeout time.Duration,
) (drainStartedAt time.Time, cleanupReadyAt time.Time, err error) {
	if drainTimeout < 0 {
		drainTimeout = 0
	}

	const query = `
UPDATE gateways
SET status = 'draining',
    drain_started_at = COALESCE(drain_started_at, NOW()),
    provisioning_status = 'deprovisioning',
    provisioning_error = NULL,
    retry_count = CASE
        WHEN provisioning_status = 'deprovisioning'
        THEN retry_count
        ELSE 0
    END,
    next_retry_at = CASE
        WHEN provisioning_status = 'deprovisioning'
             AND next_retry_at IS NOT NULL
        THEN next_retry_at
        ELSE COALESCE(drain_started_at, NOW()) + $2::interval
    END,
    locked_at = NULL,
    locked_by = NULL,
    lease_token = NULL,
    updated_at = NOW()
WHERE external_id = $1
  AND deleted_at IS NULL
  AND status <> 'decommissioned'
RETURNING drain_started_at, next_retry_at
`

	err = r.db.QueryRowContext(
		ctx,
		query,
		externalID,
		drainTimeout.String(),
	).Scan(
		&drainStartedAt,
		&cleanupReadyAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		var status string

		readErr := r.db.QueryRowContext(
			ctx,
			`SELECT status FROM gateways WHERE external_id = $1`,
			externalID,
		).Scan(&status)

		if errors.Is(readErr, sql.ErrNoRows) {
			return time.Time{}, time.Time{}, ErrGatewayNotFound
		}
		if readErr != nil {
			return time.Time{}, time.Time{},
				fmt.Errorf("read gateway after deprovision queue failure: %w", readErr)
		}

		if status == "decommissioned" {
			return time.Time{}, time.Time{}, nil
		}

		return time.Time{}, time.Time{}, ErrGatewayNotFound
	}

	if err != nil {
		return time.Time{}, time.Time{},
			fmt.Errorf("queue gateway deprovisioning: %w", err)
	}

	return drainStartedAt, cleanupReadyAt, nil
}

// MarkGatewayDecommissioned finalizes cleanup after the ALB resources and
// Docker container have all been successfully removed.
func (r *GatewayRepo) MarkGatewayDecommissioned(
	ctx context.Context,
	externalID string,
	leaseToken uuid.UUID,
) error {
	const query = `
UPDATE gateways
SET status = 'decommissioned',
    provisioning_status = 'deprovisioned',
    provisioning_error = NULL,
    public_host = '',
    public_port = '',
    public_endpoint = NULL,
    host_port = NULL,
    target_group_arn = NULL,
    listener_rule_arn = NULL,
    listener_rule_priority = NULL,
    next_retry_at = NULL,
    locked_at = NULL,
    locked_by = NULL,
    lease_token = NULL,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE external_id = $1
  AND lease_token = $2
  AND provisioning_status = 'deprovisioning'
  AND deleted_at IS NULL
`

	result, err := r.db.ExecContext(
		ctx,
		query,
		externalID,
		leaseToken,
	)
	if err != nil {
		return fmt.Errorf("mark gateway decommissioned: %w", err)
	}

	return ensureGatewayIngressUpdated(result)
}
