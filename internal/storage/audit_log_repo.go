package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"elitegate/internal/model"
	"github.com/rs/zerolog"
)

// ErrInvalidAuditLog is returned when mandatory fields are missing.
var ErrInvalidAuditLog = errors.New("audit log: action, entity_type and entity_id are required")

const (
	defaultListLimit = 100
	maxListLimit     = 1000
)

type AuditLogRepo struct {
	BaseRepo
	logger zerolog.Logger
}

func NewAuditLogRepo(db *sql.DB, logger zerolog.Logger) *AuditLogRepo {
	return &AuditLogRepo{
		BaseRepo: BaseRepo{db: db},
		logger:   logger.With().Str("repo", "audit_log").Logger(),
	}
}

// Create inserts a new audit log entry for the tenant in ctx.
// Fields Action, EntityType, and EntityID are required.
func (r *AuditLogRepo) Create(ctx context.Context, log *model.AuditLog) error {
	// --- input validation ---
	if log.Action == "" || log.EntityType == "" || log.EntityID == "" {
		return ErrInvalidAuditLog
	}

	tc, err := TenantFromContext(ctx)
	if err != nil {
		return err
	}

	if log.Status == "" {
		log.Status = "success"
	}

	const q = `
		INSERT INTO audit_logs (project_id, admin_user_id, action, entity_type, entity_id, entity_label, changes, ip_address, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::inet, $9)
		RETURNING id, created_at
	`

	if err := r.db.QueryRowContext(ctx, q,
		tc.ProjectID,
		nullUUID(log.AdminUserID),
		log.Action,
		log.EntityType,
		log.EntityID,
		log.EntityLabel,
		log.Changes,
		log.IPAddress,
		log.Status,
	).Scan(&log.ID, &log.CreatedAt); err != nil {
		return fmt.Errorf("audit_log create: %w", err)
	}

	r.logger.Info().
		Str("id", log.ID).
		Str("project_id", tc.ProjectID.String()).
		Str("action", log.Action).
		Str("entity_type", log.EntityType).
		Str("entity_id", log.EntityID).
		Msg("audit log created")

	return nil
}

// List returns a filtered, paginated page of audit logs for the tenant in ctx,
// along with the total matching row count for pagination controls.
func (r *AuditLogRepo) List(ctx context.Context, f model.AuditLogFilter) (*model.AuditLogPage, error) {
	tc, err := TenantFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Clamp / default the limit
	limit := f.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	// Build WHERE clause incrementally so unset filters don't affect the query.
	where := "WHERE al.project_id = $1"
	args := []any{tc.ProjectID}
	argN := 2

	if f.Actor != "" {
		where += fmt.Sprintf(" AND u.username ILIKE $%d", argN)
		args = append(args, "%"+f.Actor+"%")
		argN++
	}
	if f.Action != "" {
		where += fmt.Sprintf(" AND al.action = $%d", argN)
		args = append(args, f.Action)
		argN++
	}
	if f.DateFrom != nil {
		where += fmt.Sprintf(" AND al.created_at >= $%d", argN)
		args = append(args, *f.DateFrom)
		argN++
	}
	if f.DateTo != nil {
		where += fmt.Sprintf(" AND al.created_at <= $%d", argN)
		args = append(args, *f.DateTo)
		argN++
	}

	r.logger.Debug().
		Str("project_id", tc.ProjectID.String()).
		Int("limit", limit).
		Int("offset", offset).
		Msg("listing audit logs")

	countQ := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM audit_logs al
		LEFT JOIN admin_users u ON u.id = al.admin_user_id
		%s
	`, where)

	var total int
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("audit_log count: %w", err)
	}

	listQ := fmt.Sprintf(`
		SELECT al.id, al.project_id::text, COALESCE(al.admin_user_id::text, ''),
		       COALESCE(u.username, 'system'),
		       al.action, al.entity_type, al.entity_id, COALESCE(al.entity_label, ''),
		       al.changes::text, COALESCE(al.ip_address::text, ''), al.status, al.created_at
		FROM audit_logs al
		LEFT JOIN admin_users u ON u.id = al.admin_user_id
		%s
		ORDER BY al.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argN, argN+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, fmt.Errorf("audit_log list: %w", err)
	}
	defer rows.Close()

	var logs []model.AuditLog
	for rows.Next() {
		var l model.AuditLog
		if err := rows.Scan(
			&l.ID, &l.ProjectID, &l.AdminUserID, &l.Actor,
			&l.Action, &l.EntityType, &l.EntityID, &l.EntityLabel,
			&l.Changes, &l.IPAddress, &l.Status, &l.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("audit_log list scan: %w", err)
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit_log list rows: %w", err)
	}

	r.logger.Info().
		Str("project_id", tc.ProjectID.String()).
		Int("count", len(logs)).
		Int("offset", offset).
		Msg("audit logs listed")

	return &model.AuditLogPage{Logs: logs, Total: total}, nil
}

// PruneAuditLogs deletes entries older than olderThan globally (no tenant scoping).
// It is intended to be called by the background pruner running as the DB owner.
// Returns the number of rows deleted.
func (r *AuditLogRepo) PruneAuditLogs(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, errors.New("audit_log prune: olderThan must be positive")
	}

	// Use a parameterised interval to avoid string injection.
	// PostgreSQL accepts: NOW() - $1 * INTERVAL '1 second'
	const q = `
		DELETE FROM audit_logs
		WHERE created_at < NOW() - ($1 * INTERVAL '1 second')
	`
	seconds := int64(olderThan.Seconds())

	r.logger.Info().
		Int64("older_than_seconds", seconds).
		Msg("pruning audit logs")

	result, err := r.db.ExecContext(ctx, q, seconds)
	if err != nil {
		return 0, fmt.Errorf("audit_log prune: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("audit_log prune: get rows affected: %w", err)
	}

	r.logger.Info().
		Int64("rows_deleted", rowsAffected).
		Int64("older_than_seconds", seconds).
		Msg("audit log prune completed")

	return rowsAffected, nil
}

func nullUUID(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}
