package storage

import (
	"database/sql"
	"fmt"

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
		logger.Error().Msg("database DSN is empty")
		return nil, fmt.Errorf("database DSN is empty")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to open connection to DB")
		return nil, err
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)

	if err := db.Ping(); err != nil {
		logger.Error().Err(err).Msg("postgres ping failed")
		return nil, fmt.Errorf("postgres ping failed: %w", err)
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
