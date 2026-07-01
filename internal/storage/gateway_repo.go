package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

var ErrGatewayNotFound = errors.New("gateway not found")

type GatewayRepo struct {
	BaseRepo
}

func NewGatewayRepo(db *sql.DB) *GatewayRepo {
	// Ensure logs directory exists.
	if err := os.MkdirAll("logs", 0755); err != nil {
		// Ignore mkdir error and let lumberjack handle file creation failures if any
	}

	logFileWriter := &lumberjack.Logger{
		Filename:   "logs/gateway.log",
		MaxSize:    10, // megabytes
		MaxBackups: 5,
		MaxAge:     30, // days
		Compress:   true,
	}

	multi := zerolog.MultiLevelWriter(
		logFileWriter,
		zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339},
	)

	logger := zerolog.New(multi).With().Timestamp().Str("component", "gateway-repo").Logger()

	return &GatewayRepo{
		BaseRepo: BaseRepo{
			db:     db,
			logger: logger,
		},
	}
}

type GatewayRecord struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	ExternalID string `json:"external_id"`
	EndpointIP string `json:"endpoint_ip"`
	Port       string `json:"gateway_port"`
	Plan       string `json:"plan"`
	Status     string `json:"status"`
}

// Provision inserts a gateway row in "provisioning" state.
// Uses direct DB access (no RLS) because the gateway row must be written
// before the container starts — there is no active tenant session yet.
// The projectID is validated at the handler layer by the ProjectScope middleware.
func (r *GatewayRepo) Provision(ctx context.Context, externalID, projectID, plan string) error {
	r.logger.Info().
		Str("external_id", externalID).
		Str("project_id", projectID).
		Str("plan", plan).
		Msg("Provision: creating gateway database record in provisioning state")

	const q = `
		INSERT INTO gateways (external_id, project_id, endpoint_ip, gateway_port, plan, status)
		VALUES ($1, $2, '0.0.0.0', '0', $3, 'provisioning')
	`
	_, err := r.db.ExecContext(ctx, q, externalID, projectID, plan)
	if err != nil {
		return fmt.Errorf("Provision gateway: %w", err)
	}
	return nil
}

// Register updates the gateway row with the real IP and port once the container
// is running, and transitions its status to "active".
func (r *GatewayRepo) Register(ctx context.Context, externalID, ip, port string) error {
	r.logger.Info().
		Str("external_id", externalID).
		Str("endpoint_ip", ip).
		Str("port", port).
		Msg("Register: updating gateway record to active status")

	const q = `
		UPDATE gateways
		SET    endpoint_ip  = $2,
		       gateway_port = $3,
		       status       = 'active',
		       updated_at   = NOW()
		WHERE  external_id  = $1
		  AND  deleted_at   IS NULL
	`
	res, err := r.db.ExecContext(ctx, q, externalID, ip, port)
	if err != nil {
		return fmt.Errorf("Register gateway: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrGatewayNotFound
	}
	return nil
}

// UpdateStatus transitions a gateway to any valid status string.
// Used to mark a gateway as "failed" when container provisioning errors out.
func (r *GatewayRepo) UpdateStatus(ctx context.Context, externalID, status string) error {
	r.logger.Info().
		Str("external_id", externalID).
		Str("status", status).
		Msg("UpdateStatus: transitioning gateway status")

	const q = `
		UPDATE gateways
		SET    status      = $2,
		       updated_at  = NOW()
		WHERE  external_id = $1
		  AND  deleted_at  IS NULL
	`
	res, err := r.db.ExecContext(ctx, q, externalID, status)
	if err != nil {
		return fmt.Errorf("UpdateStatus gateway %s → %s: %w", externalID, status, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrGatewayNotFound
	}
	return nil
}

// Decommission soft-deletes a gateway row and marks it decommissioned.
func (r *GatewayRepo) Decommission(ctx context.Context, externalID string) error {
	r.logger.Info().
		Str("external_id", externalID).
		Msg("Decommission: marking gateway record as decommissioned and soft-deleted")

	const q = `
		UPDATE gateways
		SET    status      = 'decommissioned',
		       deleted_at  = NOW(),
		       updated_at  = NOW()
		WHERE  external_id = $1
		  AND  deleted_at  IS NULL
	`
	res, err := r.db.ExecContext(ctx, q, externalID)
	if err != nil {
		return fmt.Errorf("Decommission gateway: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrGatewayNotFound
	}
	return nil
}

// GetGatewayStatus returns the status of a gateway (even if soft-deleted/decommissioned).
// If the gateway does not exist at all, it returns ErrGatewayNotFound.
func (r *GatewayRepo) GetGatewayStatus(ctx context.Context, externalID string) (string, error) {
	var status string
	const q = `
		SELECT status
		FROM   gateways
		WHERE  external_id = $1
	`
	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, q, externalID).Scan(&status)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrGatewayNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get gateway status: %w", err)
	}
	return status, nil
}

// GetGatewayStatusPlatform retrieves the status of any gateway globally (no RLS/tenant check).
func (r *GatewayRepo) GetGatewayStatusPlatform(ctx context.Context, externalID string) (string, error) {
	var status string
	const q = `
		SELECT status
		FROM   gateways
		WHERE  external_id = $1
	`
	err := r.db.QueryRowContext(ctx, q, externalID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrGatewayNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get gateway status platform: %w", err)
	}
	return status, nil
}


// ListActive returns all non-deleted gateways in the "active" state.
// This is a global query used by the platform control plane, not scoped to a tenant.
func (r *GatewayRepo) ListActive(ctx context.Context) ([]GatewayRecord, error) {
	r.logger.Debug().Msg("ListActive: querying all active gateways")

	const q = `
		SELECT id, project_id::text, external_id, endpoint_ip, gateway_port, plan, status
		FROM   gateways
		WHERE  status     = 'active'
		  AND  deleted_at IS NULL
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("ListActive gateways: %w", err)
	}
	defer rows.Close()

	var gateways []GatewayRecord
	for rows.Next() {
		var g GatewayRecord
		if err := rows.Scan(&g.ID, &g.ProjectID, &g.ExternalID, &g.EndpointIP, &g.Port, &g.Plan, &g.Status); err != nil {
			return nil, fmt.Errorf("scan gateway row: %w", err)
		}
		gateways = append(gateways, g)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate gateway rows: %w", err)
	}

	r.logger.Debug().Int("count", len(gateways)).Msg("ListActive: query successful")
	return gateways, nil
}

// CountByStatus returns the number of non-deleted gateways grouped by
// their current status (provisioning/active/failed/decommissioned).
// Used by the platform-wide health endpoint.
func (r *GatewayRepo) CountByStatus(ctx context.Context) (map[string]int, error) {
	r.logger.Debug().Msg("CountByStatus: aggregating gateway counts by status")

	const q = `
		SELECT status, COUNT(*)
		FROM   gateways
		WHERE  deleted_at IS NULL
		GROUP BY status
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("CountByStatus gateways: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan gateway status count: %w", err)
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate gateway status counts: %w", err)
	}

	return counts, nil
}

// GetByExternalID looks up a single gateway by its human-readable external
// ID (e.g. "gw_a1b2c3d4"). Used by the platform restart and
// force-decommission endpoints to resolve a single target before acting.
func (r *GatewayRepo) GetByExternalID(ctx context.Context, externalID string) (*GatewayRecord, error) {
	r.logger.Debug().Str("external_id", externalID).Msg("GetByExternalID: looking up gateway")

	const q = `
		SELECT id, project_id::text, external_id, endpoint_ip, gateway_port, plan, status
		FROM   gateways
		WHERE  external_id = $1
		  AND  deleted_at  IS NULL
	`
	var g GatewayRecord
	err := r.db.QueryRowContext(ctx, q, externalID).Scan(
		&g.ID, &g.ProjectID, &g.ExternalID, &g.EndpointIP, &g.Port, &g.Plan, &g.Status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGatewayNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetByExternalID gateway %s: %w", externalID, err)
	}

	return &g, nil
}

// ListByProject returns all gateways for the current tenant's project.
func (r *GatewayRepo) ListByProject(ctx context.Context, projectID string) ([]GatewayRecord, error) {
	r.logger.Debug().Str("project_id", projectID).Msg("ListByProject: listing gateways for project")

	var gateways []GatewayRecord

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			SELECT id, project_id::text, external_id, endpoint_ip, gateway_port, plan, status
			FROM   gateways
			WHERE  project_id = $1
			  AND  deleted_at IS NULL
			ORDER BY created_at ASC
		`
		rows, err := tx.QueryContext(ctx, q, tc.ProjectID)
		if err != nil {
			return fmt.Errorf("query gateways: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var g GatewayRecord
			if err := rows.Scan(&g.ID, &g.ProjectID, &g.ExternalID, &g.EndpointIP, &g.Port, &g.Plan, &g.Status); err != nil {
				return fmt.Errorf("scan gateway row: %w", err)
			}
			gateways = append(gateways, g)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list gateways: %w", err)
	}

	if gateways == nil {
		gateways = []GatewayRecord{}
	}

	return gateways, nil
}

// ListAllForAdmin returns all non-deleted, active gateways belonging to projects
// where the specified admin user is a member.
// This method bypasses session-scoped RLS by executing directly on r.db, and enforces
// security by explicitly joining with the project_members table on the admin's user ID.
func (r *GatewayRepo) ListAllForAdmin(ctx context.Context, adminUserID string) ([]GatewayRecord, error) {
	r.logger.Debug().Str("admin_user_id", adminUserID).Msg("ListAllForAdmin: querying gateways for admin")

	const q = `
		SELECT g.id, g.project_id::text, g.external_id, g.endpoint_ip, g.gateway_port, g.plan, g.status
		FROM   gateways g
		JOIN   project_members pm ON g.project_id = pm.project_id
		JOIN   projects p ON g.project_id = p.id
		WHERE  pm.admin_user_id = $1
		  AND  g.status         = 'active'
		  AND  g.deleted_at     IS NULL
		  AND  p.deleted_at     IS NULL
		ORDER BY g.created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, q, adminUserID)
	if err != nil {
		return nil, fmt.Errorf("ListAllForAdmin query: %w", err)
	}
	defer rows.Close()

	var gateways []GatewayRecord
	for rows.Next() {
		var g GatewayRecord
		if err := rows.Scan(&g.ID, &g.ProjectID, &g.ExternalID, &g.EndpointIP, &g.Port, &g.Plan, &g.Status); err != nil {
			return nil, fmt.Errorf("scan gateway row: %w", err)
		}
		gateways = append(gateways, g)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate gateway rows: %w", err)
	}

	if gateways == nil {
		gateways = []GatewayRecord{}
	}

	return gateways, nil
}

