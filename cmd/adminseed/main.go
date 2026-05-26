

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Read environment variables
	username := strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
	password := os.Getenv("ADMIN_PASSWORD")
	dsn := os.Getenv("POSTGRES_DSN")
	env := os.Getenv("APP_ENV") // e.g., "development" or "production"

	// Validate required variables
	if username == "" || password == "" || dsn == "" {
		return errors.New("ADMIN_USERNAME, ADMIN_PASSWORD, and POSTGRES_DSN are required")
	}

	// Bcrypt truncates passwords over 72 bytes, so it's good to enforce a max length
	if len(password) > 72 {
		return errors.New("ADMIN_PASSWORD exceeds maximum length of 72 bytes")
	}

	// Password safety checks
	if password == "admin123" {
		if env != "development" {
			return errors.New("admin123 is only allowed when APP_ENV=development")
		}
		log.Println("warning: using development-only password")
	} else if len(password) < 12 {
		return errors.New("ADMIN_PASSWORD must be at least 12 characters outside local development")
	}

	// Open PostgreSQL connection
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	defer db.Close() // This will now execute properly because we return errors instead of log.Fatal

	// Context with timeout to prevent hanging indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify the connection is actually alive
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Hash password using bcrypt
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert admin user
	const q = `
	INSERT INTO admin_users
	(username, password_hash)
	VALUES ($1, $2)
	ON CONFLICT (username)
	DO NOTHING
	RETURNING id
	`

	var id string
	err = db.QueryRowContext(ctx, q, username, string(hash)).Scan(&id)

	// User already exists
	if errors.Is(err, sql.ErrNoRows) {
		fmt.Printf("admin user %q already exists\n", username)
		return nil
	}

	// Handle other DB errors
	if err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}

	// Success message
	fmt.Printf("admin user %q created with id %s\n", username, id)
	return nil
}