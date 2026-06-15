package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"elitegate/internal/auth"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	ErrAPIKeyNotFound = errors.New("api key not found")
	ErrAPIKeyRevoked  = errors.New("api key has been revoked")
	ErrAPIKeyExpired  = errors.New("api key has expired")
)

// ApiKeyRecord mirrors the api_keys table columns returned to callers.
type ApiKeyRecord struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Name      string     `json:"name"`
	KeyHash   string     `json:"-"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// ─── Repository ──────────────────────────────────────────────────────────────

// ApiKeyRepo manages api_keys database operations.
// Tenant queries use RLS; FindByHash bypasses RLS for authentication.
type ApiKeyRepo struct {
	BaseRepo
	logger zerolog.Logger
}

// NewApiKeyRepo creates an ApiKeyRepo with rotating file and console logs.
func NewApiKeyRepo(db *sql.DB) *ApiKeyRepo {
	if err := os.MkdirAll("logs", 0o755); err != nil {
		// Non-fatal — lumberjack will attempt the file itself.
	}

	fileWriter := &lumberjack.Logger{
		Filename:   "logs/api_key.log",
		MaxSize:    10,   // MB before rotation
		MaxBackups: 5,    // number of old files to keep
		MaxAge:     30,   // days
		Compress:   true, // gzip rotated files
	}

	multi := zerolog.MultiLevelWriter(
		fileWriter,
		zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339},
	)

	logger := zerolog.New(multi).
		With().
		Timestamp().
		Str("component", "api-key-repo").
		Logger()

	return &ApiKeyRepo{
		BaseRepo: BaseRepo{db: db},
		logger:   logger,
	}
}

// Create an API key for a tenant.
// Only the key hash is stored in the database.
func (r *ApiKeyRepo) Create(
	ctx context.Context,
	name string,
	rawKey string,
	expiresAt *time.Time,
) (*ApiKeyRecord, error) {
	r.logger.Info().
		Str("name", name).
		Msg("Create: hashing raw key and inserting api_key record")

	keyHash := hashRawKey(rawKey)

	rec := &ApiKeyRecord{
		Name:      name,
		KeyHash:   keyHash,
		Status:    "active",
		ExpiresAt: expiresAt,
	}

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		rec.ProjectID = tc.ProjectID.String()

		const q = `
			INSERT INTO api_keys (project_id, name, key_hash, expires_at)
			VALUES ($1, $2, $3, $4)
			RETURNING id, status, created_at, updated_at
		`
		return tx.QueryRowContext(ctx, q, tc.ProjectID, name, keyHash, expiresAt).
			Scan(&rec.ID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
	})
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("name", name).
			Msg("Create: failed to insert api_key")
		return nil, fmt.Errorf("api_key create: %w", err)
	}

	r.logger.Info().
		Str("id", rec.ID).
		Str("project_id", rec.ProjectID).
		Str("name", name).
		Msg("Create: api_key created successfully")

	return rec, nil
}

// Revoke a tenant API key.
// Returns ErrAPIKeyNotFound if the key does not exist.
func (r *ApiKeyRepo) Revoke(ctx context.Context, keyID string) error {
	r.logger.Info().
		Str("key_id", keyID).
		Msg("Revoke: revoking api_key")

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			UPDATE api_keys
			SET    status     = 'revoked',
			       deleted_at = NOW(),
			       updated_at = NOW()
			WHERE  id         = $1
			  AND  project_id = $2
			  AND  deleted_at IS NULL
		`
		res, err := tx.ExecContext(ctx, q, keyID, tc.ProjectID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrAPIKeyNotFound
		}
		return nil
	})
	if errors.Is(err, ErrAPIKeyNotFound) {
		r.logger.Warn().
			Str("key_id", keyID).
			Msg("Revoke: api_key not found or already revoked")
		return ErrAPIKeyNotFound
	}
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("key_id", keyID).
			Msg("Revoke: database update failed")
		return fmt.Errorf("api_key revoke %s: %w", keyID, err)
	}

	r.logger.Info().
		Str("key_id", keyID).
		Msg("Revoke: api_key revoked successfully")
	return nil
}

// Rotate(Replace) an API key by revoking the old key and creating a new one.
// The caller must keep the new raw key.
func (r *ApiKeyRepo) Rotate(
	ctx context.Context,
	oldKeyID string,
	rawNewKey string,
) (*ApiKeyRecord, error) {
	r.logger.Info().
		Str("old_key_id", oldKeyID).
		Msg("Rotate: rotating api_key")

	newHash := hashRawKey(rawNewKey)

	var newRec ApiKeyRecord

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		// Step 1 — revoke the old key and retrieve its metadata.
		const qRevoke = `
			UPDATE api_keys
			SET    status     = 'revoked',
			       deleted_at = NOW(),
			       updated_at = NOW()
			WHERE  id         = $1
			  AND  project_id = $2
			  AND  deleted_at IS NULL
			RETURNING name, expires_at
		`
		var name string
		var expiresAt *time.Time
		if err := tx.QueryRowContext(ctx, qRevoke, oldKeyID, tc.ProjectID).
			Scan(&name, &expiresAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAPIKeyNotFound
			}
			return fmt.Errorf("revoke old key: %w", err)
		}

		r.logger.Debug().
			Str("old_key_id", oldKeyID).
			Str("name", name).
			Msg("Rotate: old key revoked, inserting replacement")

		// Step 2 — insert the replacement key.
		newRec.ProjectID = tc.ProjectID.String()
		newRec.Name = name
		newRec.KeyHash = newHash
		newRec.Status = "active"
		newRec.ExpiresAt = expiresAt

		const qInsert = `
			INSERT INTO api_keys (project_id, name, key_hash, expires_at)
			VALUES ($1, $2, $3, $4)
			RETURNING id, status, created_at, updated_at
		`
		return tx.QueryRowContext(ctx, qInsert, tc.ProjectID, name, newHash, expiresAt).
			Scan(&newRec.ID, &newRec.Status, &newRec.CreatedAt, &newRec.UpdatedAt)
	})
	if errors.Is(err, ErrAPIKeyNotFound) {
		r.logger.Warn().
			Str("old_key_id", oldKeyID).
			Msg("Rotate: old api_key not found")
		return nil, ErrAPIKeyNotFound
	}
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("old_key_id", oldKeyID).
			Msg("Rotate: rotation transaction failed")
		return nil, fmt.Errorf("api_key rotate: %w", err)
	}

	r.logger.Info().
		Str("old_key_id", oldKeyID).
		Str("new_key_id", newRec.ID).
		Str("project_id", newRec.ProjectID).
		Msg("Rotate: api_key rotated successfully")

	return &newRec, nil
}

// ─── Read operations (tenant-scoped) ─────────────────────────────────────────

// Get an active API key by ID.
// Returns ErrAPIKeyNotFound if not found.
func (r *ApiKeyRepo) GetByID(ctx context.Context, keyID string) (*ApiKeyRecord, error) {
	r.logger.Debug().
		Str("key_id", keyID).
		Msg("GetByID: fetching api_key")

	var rec ApiKeyRecord

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			SELECT id, project_id::text, name, key_hash, status,
			       expires_at, created_at, updated_at
			FROM   api_keys
			WHERE  id         = $1
			  AND  project_id = $2
			  AND  deleted_at IS NULL
		`
		return tx.QueryRowContext(ctx, q, keyID, tc.ProjectID).Scan(
			&rec.ID, &rec.ProjectID, &rec.Name, &rec.KeyHash,
			&rec.Status, &rec.ExpiresAt, &rec.CreatedAt, &rec.UpdatedAt,
		)
	})
	if errors.Is(err, sql.ErrNoRows) {
		r.logger.Warn().Str("key_id", keyID).Msg("GetByID: api_key not found")
		return nil, ErrAPIKeyNotFound
	}
	if err != nil {
		r.logger.Error().Err(err).Str("key_id", keyID).Msg("GetByID: query failed")
		return nil, fmt.Errorf("api_key get %s: %w", keyID, err)
	}

	r.logger.Debug().
		Str("key_id", keyID).
		Str("status", rec.Status).
		Msg("GetByID: api_key fetched successfully")

	return &rec, nil
}

// List all active API keys for the current tenant.
// Returns an empty slice if none exist.
func (r *ApiKeyRepo) ListAll(ctx context.Context) ([]ApiKeyRecord, error) {
	r.logger.Debug().Msg("ListAll: listing api_keys for tenant")

	var keys []ApiKeyRecord

	err := r.withTenantTx(ctx, func(tx *sql.Tx) error {
		tc, err := TenantFromContext(ctx)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		const q = `
			SELECT id, project_id::text, name, key_hash, status,
			       expires_at, created_at, updated_at
			FROM   api_keys
			WHERE  project_id = $1
			  AND  deleted_at IS NULL
			ORDER BY created_at DESC
		`
		rows, err := tx.QueryContext(ctx, q, tc.ProjectID)
		if err != nil {
			return fmt.Errorf("query api_keys: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var rec ApiKeyRecord
			if err := rows.Scan(
				&rec.ID, &rec.ProjectID, &rec.Name, &rec.KeyHash,
				&rec.Status, &rec.ExpiresAt, &rec.CreatedAt, &rec.UpdatedAt,
			); err != nil {
				return fmt.Errorf("scan api_key row: %w", err)
			}
			keys = append(keys, rec)
		}
		return rows.Err()
	})
	if err != nil {
		r.logger.Error().Err(err).Msg("ListAll: query failed")
		return nil, fmt.Errorf("api_key list: %w", err)
	}

	// Always return an empty slice rather than nil so JSON encodes as [].
	if keys == nil {
		keys = []ApiKeyRecord{}
	}

	r.logger.Debug().Int("count", len(keys)).Msg("ListAll: api_keys listed successfully")
	return keys, nil
}

// Global API key lookup (bypasses tenant RLS)─────────────────────────────────────────

// Find an active API key by its hash.
// Used during authentication before the tenant is identified.
func (r *ApiKeyRepo) FindByHash(
	ctx context.Context,
	hash string,
) (*auth.APIKeyRecord, error) {
	r.logger.Debug().
		Str("hash_prefix", safePrefix(hash, 8)).
		Msg("FindByHash: global api_key lookup")

	const q = `
		SELECT project_id::text, status, expires_at, deleted_at
		FROM   api_keys
		WHERE  key_hash = $1
	`

	var (
		projectID string
		status    string
		expiresAt sql.NullTime
		deletedAt sql.NullTime
	)

	err := r.db.QueryRowContext(ctx, q, hash).
		Scan(&projectID, &status, &expiresAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		r.logger.Debug().
			Str("hash_prefix", safePrefix(hash, 8)).
			Msg("FindByHash: api_key not found")
		return nil, nil // cache miss → caller treats as invalid
	}
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("hash_prefix", safePrefix(hash, 8)).
			Msg("FindByHash: database query failed")
		return nil, fmt.Errorf("api_key find by hash: %w", err)
	}

	// Soft-deleted keys are treated as revoked.
	var revokedAt *time.Time
	if deletedAt.Valid {
		revokedAt = &deletedAt.Time
		r.logger.Debug().
			Str("hash_prefix", safePrefix(hash, 8)).
			Msg("FindByHash: api_key is soft-deleted (revoked)")
	}

	// Expired keys: still return the record so RedisKeyStore can decide,
	// but log a warning to aid debugging.
	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		r.logger.Warn().
			Str("hash_prefix", safePrefix(hash, 8)).
			Str("project_id", projectID).
			Time("expired_at", expiresAt.Time).
			Msg("FindByHash: api_key is expired")
		// Return a record with a revokedAt set so the store rejects it.
		now := time.Now()
		revokedAt = &now
	}

	r.logger.Debug().
		Str("project_id", projectID).
		Str("status", status).
		Msg("FindByHash: api_key found")

	return &auth.APIKeyRecord{
		ClientID:  projectID, // project UUID is the client identity on the gateway
		RevokedAt: revokedAt,
	}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// Hash an API key using SHA-256.
func hashRawKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return fmt.Sprintf("%x", h)
}

// Return the first n characters of a string.
// Used to hide the full hash in logs.
func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
