package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// GatewayProvisioningJob contains the information required by the Worker
// to expose one dedicated gateway through the ALB.
type GatewayProvisioningJob struct {
	ExternalID           string
	ProjectID            string
	HostPort             int
	TargetGroupARN       string
	ListenerRuleARN      string
	ListenerRulePriority int
	ProvisioningStatus   string
	RetryCount           int
	LeaseToken           uuid.UUID
}

// ClaimNextGatewayIngressJob safely claims one eligible gateway job.
// SKIP LOCKED prevents multiple workers from processing the same gateway.
func (r *GatewayRepo) ClaimNextGatewayIngressJob(
	ctx context.Context,
	workerID string,
	lockTimeout time.Duration,
) (*GatewayProvisioningJob, error) {
	if lockTimeout <= 0 {
		lockTimeout = 5 * time.Minute
	}

	lockExpiredBefore := time.Now().Add(-lockTimeout)
	leaseToken := uuid.New()

	const query = `
WITH next_job AS (
SELECT external_id
FROM gateways
WHERE deleted_at IS NULL
  AND provisioning_status IN (
      'container_ready',
      'creating_target_group',
      'registering_target',
      'waiting_for_target_health',
      'creating_listener_rule',
      'deprovisioning'
  )
  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
  AND (locked_at IS NULL OR locked_at <= $1)
ORDER BY next_retry_at ASC NULLS FIRST, created_at ASC
FOR UPDATE SKIP LOCKED
LIMIT 1
)
UPDATE gateways g
SET locked_at  = NOW(),
    locked_by  = $2,
    lease_token = $3,
    updated_at = NOW()
FROM next_job
WHERE g.external_id = next_job.external_id
RETURNING
g.external_id,
g.project_id::text,
COALESCE(g.host_port, 0),
COALESCE(g.target_group_arn, ''),
COALESCE(g.listener_rule_arn, ''),
COALESCE(g.listener_rule_priority, 0),
g.provisioning_status,
g.retry_count
`

	job := &GatewayProvisioningJob{
		LeaseToken: leaseToken,
	}

	err := r.db.QueryRowContext(
		ctx,
		query,
		lockExpiredBefore,
		workerID,
		leaseToken,
	).Scan(
		&job.ExternalID,
		&job.ProjectID,
		&job.HostPort,
		&job.TargetGroupARN,
		&job.ListenerRuleARN,
		&job.ListenerRulePriority,
		&job.ProvisioningStatus,
		&job.RetryCount,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim next gateway ingress job: %w", err)
	}

	return job, nil
}
