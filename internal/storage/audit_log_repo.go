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
		r.logger.Error().Err(err).Msg("create: missing tenant context")
		return err
	}

	const q = `
		INSERT INTO audit_logs (project_id, admin_user_id, action, entity_type, entity_id, changes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	if err := r.db.QueryRowContext(ctx, q,
		tc.ProjectID,
		nullUUID(log.AdminUserID),
		log.Action,
		log.EntityType,
		log.EntityID,
		log.Changes,
	).Scan(&log.ID, &log.CreatedAt); err != nil {
		r.logger.Error().
			Err(err).
			Str("project_id", tc.ProjectID.String()).
			Str("action", log.Action).
			Str("entity_type", log.EntityType).
			Str("entity_id", log.EntityID).
			Msg("create: failed to insert audit log")
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

// List returns paginated audit logs for the tenant in ctx.
// Pass a zero-value AuditLogFilter to use defaults (limit=100, offset=0).
func (r *AuditLogRepo) List(ctx context.Context, f model.AuditLogFilter) ([]model.AuditLog, error) {
	tc, err := TenantFromContext(ctx)
	if err != nil {
		r.logger.Error().Err(err).Msg("list: missing tenant context")
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

	const q = `
		SELECT id, project_id::text, COALESCE(admin_user_id::text, ''),
		       action, entity_type, entity_id, changes::text, created_at
		FROM audit_logs
		WHERE project_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	r.logger.Debug().
		Str("project_id", tc.ProjectID.String()).
		Int("limit", limit).
		Int("offset", offset).
		Msg("listing audit logs")

	rows, err := r.db.QueryContext(ctx, q, tc.ProjectID, limit, offset)
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("project_id", tc.ProjectID.String()).
			Msg("list: query failed")
		return nil, fmt.Errorf("audit_log list: %w", err)
	}
	defer rows.Close()

	var logs []model.AuditLog
	for rows.Next() {
		var l model.AuditLog
		if err := rows.Scan(
			&l.ID, &l.ProjectID, &l.AdminUserID,
			&l.Action, &l.EntityType, &l.EntityID,
			&l.Changes, &l.CreatedAt,
		); err != nil {
			r.logger.Error().Err(err).Msg("list: scan failed")
			return nil, fmt.Errorf("audit_log list scan: %w", err)
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		r.logger.Error().Err(err).Msg("list: rows iteration error")
		return nil, fmt.Errorf("audit_log list rows: %w", err)
	}

	r.logger.Info().
		Str("project_id", tc.ProjectID.String()).
		Int("count", len(logs)).
		Int("offset", offset).
		Msg("audit logs listed")

	return logs, nil
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
		r.logger.Error().
			Err(err).
			Int64("older_than_seconds", seconds).
			Msg("prune: delete query failed")
		return 0, fmt.Errorf("audit_log prune: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		// Non-fatal; log and return 0 count.
		r.logger.Warn().Err(err).Msg("prune: could not retrieve rows affected")
		return 0, nil
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
