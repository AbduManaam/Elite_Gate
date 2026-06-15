package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"elitegate/internal/model"

	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrSlugConflict    = errors.New("slug already exists")
)

type ProjectRepo struct {
	BaseRepo
}

func NewProjectRepo(db *sql.DB, logger zerolog.Logger) *ProjectRepo {
	return &ProjectRepo{BaseRepo{db: db, logger: logger}}
}

// Returns true if a duplicate entry already exists.
func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// Create a project and assign the creator as its owner.
// Both operations run in a single transaction.
func (r *ProjectRepo) Create(ctx context.Context, p *model.Project) error {
	r.logger.Info().Str("slug", p.Slug).Msg("Create: initiating project creation")

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		r.logger.Error().Err(err).Msg("Create: failed to start database transaction")
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Rollback only if we have not committed yet
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	const q = `
		INSERT INTO projects (name, slug, description, owner_id, plan)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, is_active, created_at, updated_at
	`
	err = tx.QueryRowContext(ctx, q, p.Name, p.Slug, p.Description, p.OwnerID, p.Plan).
		Scan(&p.ID, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {

		// Return a clear error if slug is already taken
		if isUniqueViolation(err) {
			r.logger.Warn().Str("slug", p.Slug).Msg("Create: slug conflict detected")
			return ErrSlugConflict
		}
		r.logger.Error().Err(err).Str("slug", p.Slug).Msg("Create: failed to insert project row")
		return fmt.Errorf("insert project: %w", err)
	}

	// 2. Automatically assign owner membership for the creator
	const qMem = `
		INSERT INTO project_members (project_id, admin_user_id, role)
		VALUES ($1, $2, 'owner')
	`
	_, err = tx.ExecContext(ctx, qMem, p.ID, p.OwnerID)
	if err != nil {
		r.logger.Error().Err(err).Str("project_id", p.ID).Str("owner_id", p.OwnerID).Msg("Create: failed to assign owner membership")
		return fmt.Errorf("assign owner membership: %w", err)
	}

	if err := tx.Commit(); err != nil {
		r.logger.Error().Err(err).Msg("Create: failed to commit transaction")
		return fmt.Errorf("commit transaction: %w", err)
	}

	committed = true
	r.logger.Info().Str("project_id", p.ID).Str("slug", p.Slug).Msg("Create: project created successfully with owner membership assigned")
	return nil
}

// List all projects for a user.
func (r *ProjectRepo) ListForUser(ctx context.Context, userID string) ([]model.Project, error) {
	r.logger.Debug().Str("user_id", userID).Msg("ListForUser: fetching user projects")

	const q = `
		SELECT
			p.id,
			p.name,
	    	p.slug,
			COALESCE(p.description, ''),
			p.owner_id,
			p.is_active,
			p.plan,
			p.created_at,
			p.updated_at
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id
		WHERE pm.admin_user_id = $1
		  AND p.deleted_at IS NULL
		ORDER BY p.name ASC
	`
	rows, err := r.db.QueryContext(ctx, q, userID)
	if err != nil {
		r.logger.Error().Err(err).Str("user_id", userID).Msg("ListForUser: query failed")
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close() //ensures the database cursor is always closed when the function finishes, preventing connection leaks that can crash your application.

	// Initialise as empty slice so JSON returns [] not null
	projects := make([]model.Project, 0)

	//This is the process of taking all the project records fetched from the database table and converting them into individual Go objects (structs) one by one.
	for rows.Next() {
		var p model.Project
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Slug, &p.Description,
			&p.OwnerID, &p.IsActive, &p.Plan,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			r.logger.Error().Err(err).Msg("ListForUser: failed to scan project row")
			return nil, fmt.Errorf("scan project row: %w", err)
		}
		projects = append(projects, p)
	}

	// Check for errors that occurred during iteration
	if err := rows.Err(); err != nil {
		r.logger.Error().Err(err).Msg("ListForUser: row iteration error")
		return nil, fmt.Errorf("iterate project rows: %w", err)
	}

	r.logger.Debug().Str("user_id", userID).Int("count", len(projects)).Msg("ListForUser: user projects fetched successfully")
	return projects, nil
}

// Update a project's details.
// Returns ErrProjectNotFound if the project does not exist.
func (r *ProjectRepo) Update(ctx context.Context, id string, p *model.Project) error {
	r.logger.Info().Str("project_id", id).Msg("Update: initiating project update")

	const q = `
		UPDATE projects
		SET
			name        = $2,
			description = $3,
			plan        = $4,
			updated_at  = NOW()
		WHERE id = $1
		  AND deleted_at IS NULL
		RETURNING updated_at
	`
	err := r.db.QueryRowContext(ctx, q,
		id, p.Name, p.Description, p.Plan,
	).Scan(&p.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		r.logger.Warn().Str("project_id", id).Msg("Update: project not found or soft-deleted")
		return ErrProjectNotFound
	}
	if err != nil {
		r.logger.Error().Err(err).Str("project_id", id).Msg("Update: failed to update project row")
		return fmt.Errorf("update project: %w", err)
	}

	r.logger.Info().Str("project_id", id).Msg("Update: project updated successfully")
	return nil
}

// Soft-delete a project, API keys, and routes.
// Keeps related data consistent.
func (r *ProjectRepo) Delete(ctx context.Context, id string) error {
	r.logger.Info().Str("project_id", id).Msg("Delete: initiating project deletion with cascades")

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		r.logger.Error().Err(err).Msg("Delete: failed to start database transaction")
		return fmt.Errorf("begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	// 1. Soft-delete all API keys under this project
	_, err = tx.ExecContext(ctx, `
		UPDATE api_keys
		SET deleted_at = NOW(), updated_at = NOW(), status = 'revoked'
		WHERE project_id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		r.logger.Error().Err(err).Str("project_id", id).Msg("Delete: failed to soft-delete associated api_keys")
		return fmt.Errorf("soft-delete api_keys: %w", err)
	}

	// 2. Soft-delete all routes under this project
	_, err = tx.ExecContext(ctx, `
		UPDATE routes
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE project_id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		r.logger.Error().Err(err).Str("project_id", id).Msg("Delete: failed to soft-delete associated routes")
		return fmt.Errorf("soft-delete routes: %w", err)
	}

	// 3. Soft-delete all upstreams under this project
	_, err = tx.ExecContext(ctx, `
		UPDATE upstreams
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE project_id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		r.logger.Error().Err(err).Str("project_id", id).Msg("Delete: failed to soft-delete associated upstreams")
		return fmt.Errorf("soft-delete upstreams: %w", err)
	}

	// 4. Finally soft-delete the project itself
	res, err := tx.ExecContext(ctx, `
		UPDATE projects
		SET deleted_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		r.logger.Error().Err(err).Str("project_id", id).Msg("Delete: failed to soft-delete project row")
		return fmt.Errorf("soft-delete project: %w", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		r.logger.Warn().Str("project_id", id).Msg("Delete: project not found or already deleted")
		return ErrProjectNotFound
	}

	if err := tx.Commit(); err != nil {
		r.logger.Error().Err(err).Msg("Delete: failed to commit transaction")
		return fmt.Errorf("commit transaction: %w", err)
	}

	committed = true
	r.logger.Info().Str("project_id", id).Msg("Delete: project and all its child resources soft-deleted successfully")
	return nil
}
