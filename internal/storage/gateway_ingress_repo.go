package storage

import (
	"context"
	"fmt"
	"strconv"
)

// MarkContainerReady saves the Docker container endpoint and queues the
// gateway for ALB target-group and listener-rule provisioning.
func (r *GatewayRepo) MarkContainerReady(
	ctx context.Context,
	externalID string,
	endpointIP string,
	gatewayPort string,
	publicHost string,
	publicPort string,
) error {
	hostPort, err := strconv.Atoi(publicPort)
	if err != nil || hostPort < 1 || hostPort > 65535 {
		return fmt.Errorf("invalid Docker host port %q", publicPort)
	}

	const q = `
        UPDATE gateways
        SET endpoint_ip          = $2,
            gateway_port        = $3,
            public_host         = $4,
            public_port         = $5,
            host_port           = $6,
            status              = 'provisioning',
            provisioning_status = 'container_ready',
            provisioning_error  = NULL,
            retry_count         = 0,
            next_retry_at       = NOW(),
            updated_at          = NOW()
        WHERE external_id = $1
          AND deleted_at IS NULL
    `

	result, err := r.db.ExecContext(
		ctx,
		q,
		externalID,
		endpointIP,
		gatewayPort,
		publicHost,
		publicPort,
		hostPort,
	)
	if err != nil {
		return fmt.Errorf("mark gateway container ready: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected gateway rows: %w", err)
	}
	if affected == 0 {
		return ErrGatewayNotFound
	}

	return nil
}
