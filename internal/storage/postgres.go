package storage

import (
	"database/sql"
	"fmt"
	"time"

	"elitegate/internal/config"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
)

func NewPostgres(logger zerolog.Logger, cfg config.DatabaseConfig) (*sql.DB, error) {
	dsn := cfg.DSN
	if dsn == "" {
		return nil, fmt.Errorf("database DSN is empty")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database connection: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)

	if err := pingWithRetry(db, logger); err != nil {
		return nil, err
	}

	logger.Info().Msg("Connected to Postgres, running migrations...")

	// ── Run Migrations ──
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("could not create postgres migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://migrations",
		"postgres", driver)
	if err != nil {
		return nil, fmt.Errorf("could not create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	logger.Info().Msg("Database migrations applied successfully!")

	return db, nil
}

// pingWithRetry retries the initial connection a few times with linear backoff.
// Covers transient DNS propagation delays and postgres not yet accepting
// connections despite passing its healthcheck a moment earlier.
func pingWithRetry(db *sql.DB, logger zerolog.Logger) error {
	const maxAttempts = 5
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = db.Ping(); err == nil {
			return nil
		}
		logger.Warn().Err(err).Int("attempt", attempt).Int("max_attempts", maxAttempts).
			Msg("postgres ping failed, retrying")
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt) * time.Second) // 1s, 2s, 3s, 4s
		}
	}
	return fmt.Errorf("postgres ping failed after %d attempts: %w", maxAttempts, err)
}
