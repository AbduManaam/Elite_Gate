package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

func TestProjectJWTConfigIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	adminDB, err := sql.Open("postgres", testAdminDSN())
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	defer adminDB.Close()

	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("ping admin database: %v", err)
	}

	// Keep the existing EliteGate local/CI application-role convention.
	_, err = adminDB.ExecContext(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_roles WHERE rolname = 'gate_app'
			) THEN
				CREATE ROLE gate_app
					WITH LOGIN PASSWORD 'gate_app_password';
			END IF;
		END
		$$;

		ALTER ROLE gate_app
			WITH LOGIN PASSWORD 'gate_app_password';
	`)
	if err != nil {
		t.Fatalf("ensure gate_app role: %v", err)
	}

	appDB, err := sql.Open("postgres", testAppDSN())
	if err != nil {
		t.Fatalf("open app database: %v", err)
	}
	defer appDB.Close()

	if err := appDB.PingContext(ctx); err != nil {
		t.Fatalf("ping app database: %v", err)
	}

	// Grant the table only to whichever non-superuser role this test uses.
	var appRole string
	if err := appDB.QueryRowContext(
		ctx,
		`SELECT current_user`,
	).Scan(&appRole); err != nil {
		t.Fatalf("read app database role: %v", err)
	}

	grantSQL := fmt.Sprintf(
		`GRANT SELECT, INSERT, UPDATE, DELETE
		 ON project_jwt_configs TO %s`,
		pq.QuoteIdentifier(appRole),
	)

	if _, err := adminDB.ExecContext(ctx, grantSQL); err != nil {
		t.Fatalf("grant JWT config permissions: %v", err)
	}

	suffix := uuid.NewString()[:8]

	userA := uuid.New()
	userB := uuid.New()

	projectA := uuid.New()
	projectB := uuid.New()

	_, err = adminDB.ExecContext(ctx, `
		INSERT INTO admin_users (
			id,
			username,
			password_hash,
			email
		)
		VALUES
			($1, $2, 'hash', $3),
			($4, $5, 'hash', $6)
	`,
		userA,
		"jwt_user_a_"+suffix,
		"jwt-user-a-"+suffix+"@elitegate.local",

		userB,
		"jwt_user_b_"+suffix,
		"jwt-user-b-"+suffix+"@elitegate.local",
	)
	if err != nil {
		t.Fatalf("insert test users: %v", err)
	}

	_, err = adminDB.ExecContext(ctx, `
		INSERT INTO projects (
			id,
			name,
			slug,
			owner_id
		)
		VALUES
			($1, 'JWT Project A', $2, $3),
			($4, 'JWT Project B', $5, $6)
	`,
		projectA,
		"jwt-a-"+suffix,
		userA,

		projectB,
		"jwt-b-"+suffix,
		userB,
	)
	if err != nil {
		_, _ = adminDB.ExecContext(
			ctx,
			`DELETE FROM admin_users WHERE id IN ($1, $2)`,
			userA,
			userB,
		)

		t.Fatalf("insert test projects: %v", err)
	}

	defer func() {
		_, _ = adminDB.ExecContext(
			ctx,
			`DELETE FROM projects WHERE id IN ($1, $2)`,
			projectA,
			projectB,
		)

		_, _ = adminDB.ExecContext(
			ctx,
			`DELETE FROM admin_users WHERE id IN ($1, $2)`,
			userA,
			userB,
		)
	}()

	_, err = adminDB.ExecContext(ctx, `
		INSERT INTO project_members (
			project_id,
			admin_user_id,
			role
		)
		VALUES
			($1, $2, 'owner'),
			($3, $4, 'owner')
	`,
		projectA,
		userA,
		projectB,
		userB,
	)
	if err != nil {
		t.Fatalf("insert project memberships: %v", err)
	}

	repo := storage.NewProjectJWTConfigRepo(
		appDB,
		zerolog.Nop(),
	)

	tenantA := storage.WithTenantContext(
		ctx,
		storage.TenantContext{
			ProjectID: projectA,
			UserID:    userA,
			UserRole:  "owner",
		},
	)

	tenantB := storage.WithTenantContext(
		ctx,
		storage.TenantContext{
			ProjectID: projectB,
			UserID:    userB,
			UserRole:  "owner",
		},
	)

	issuerA := "https://issuer-a.example"
	issuerB := "https://issuer-b.example"

	configA := &model.ProjectJWTConfig{
		Enabled:   true,
		Algorithm: model.JWTAlgorithmHS256,

		SecretARN: "arn:aws:secretsmanager:test:000000000000:secret:project-a",

		SecretVersionID: "version-a-1",

		Issuer:    &issuerA,
		Audiences: []string{"yumzy-api"},

		SubjectClaim: "sub",
		RoleClaim:    "role",
		ScopesClaim:  "scope",

		ClockSkewSeconds: 30,
	}

	configB := &model.ProjectJWTConfig{
		Enabled:   true,
		Algorithm: model.JWTAlgorithmHS256,

		SecretARN: "arn:aws:secretsmanager:test:000000000000:secret:project-b",

		SecretVersionID: "version-b-1",

		Issuer:    &issuerB,
		Audiences: []string{"company-b-api"},

		SubjectClaim: "sub",
		RoleClaim:    "role",
		ScopesClaim:  "scope",

		ClockSkewSeconds: 30,
	}

	// -----------------------------------------------------
	// Create isolated configs
	// -----------------------------------------------------

	if err := repo.Upsert(tenantA, configA); err != nil {
		t.Fatalf("create project A JWT config: %v", err)
	}

	if configA.ConfigVersion != 1 {
		t.Fatalf(
			"project A config version = %d, want 1",
			configA.ConfigVersion,
		)
	}

	if err := repo.Upsert(tenantB, configB); err != nil {
		t.Fatalf("create project B JWT config: %v", err)
	}

	if configB.ConfigVersion != 1 {
		t.Fatalf(
			"project B config version = %d, want 1",
			configB.ConfigVersion,
		)
	}

	// -----------------------------------------------------
	// A sees only A
	// -----------------------------------------------------

	gotA, err := repo.Get(tenantA)
	if err != nil {
		t.Fatalf("get project A JWT config: %v", err)
	}

	if gotA.ProjectID != projectA.String() {
		t.Fatalf(
			"project A received project %s",
			gotA.ProjectID,
		)
	}

	if gotA.SecretVersionID != "version-a-1" {
		t.Fatalf(
			"project A received incorrect secret version: %s",
			gotA.SecretVersionID,
		)
	}

	// -----------------------------------------------------
	// B sees only B
	// -----------------------------------------------------

	gotB, err := repo.Get(tenantB)
	if err != nil {
		t.Fatalf("get project B JWT config: %v", err)
	}

	if gotB.ProjectID != projectB.String() {
		t.Fatalf(
			"project B received project %s",
			gotB.ProjectID,
		)
	}

	if gotB.SecretVersionID != "version-b-1" {
		t.Fatalf(
			"project B received incorrect secret version: %s",
			gotB.SecretVersionID,
		)
	}

	// -----------------------------------------------------
	// Prove PostgreSQL RLS itself blocks B → A
	// -----------------------------------------------------

	tx, err := appDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin RLS transaction: %v", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`
			SELECT
				set_config('app.project_id', $1, TRUE),
				set_config('app.current_user_id', $2, TRUE)
		`,
		projectB.String(),
		userB.String(),
	); err != nil {
		_ = tx.Rollback()
		t.Fatalf("set project B RLS context: %v", err)
	}

	var visibleProjectARows int

	err = tx.QueryRowContext(
		ctx,
		`
			SELECT COUNT(*)
			FROM project_jwt_configs
			WHERE project_id = $1
		`,
		projectA,
	).Scan(&visibleProjectARows)

	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("query project A from tenant B: %v", err)
	}

	if visibleProjectARows != 0 {
		_ = tx.Rollback()

		t.Fatalf(
			"RLS leak: project B can see %d project A rows",
			visibleProjectARows,
		)
	}

	if err := tx.Rollback(); err != nil &&
		!errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("rollback RLS transaction: %v", err)
	}

	// -----------------------------------------------------
	// Updating A increments only A's config version
	// -----------------------------------------------------

	configA.SecretVersionID = "version-a-2"

	if err := repo.Upsert(tenantA, configA); err != nil {
		t.Fatalf("update project A JWT config: %v", err)
	}

	if configA.ConfigVersion != 2 {
		t.Fatalf(
			"project A config version = %d, want 2",
			configA.ConfigVersion,
		)
	}

	gotB, err = repo.Get(tenantB)
	if err != nil {
		t.Fatalf("get project B after A update: %v", err)
	}

	if gotB.ConfigVersion != 1 {
		t.Fatalf(
			"project B config version changed to %d",
			gotB.ConfigVersion,
		)
	}

	if gotB.SecretVersionID != "version-b-1" {
		t.Fatalf(
			"project B secret version changed to %s",
			gotB.SecretVersionID,
		)
	}

	// -----------------------------------------------------
	// Deleting A must not delete B
	// -----------------------------------------------------

	if err := repo.Delete(tenantA); err != nil {
		t.Fatalf("delete project A JWT config: %v", err)
	}

	_, err = repo.Get(tenantA)

	if !errors.Is(
		err,
		storage.ErrProjectJWTConfigNotFound,
	) {
		t.Fatalf(
			"get deleted A error = %v, want ErrProjectJWTConfigNotFound",
			err,
		)
	}

	if _, err := repo.Get(tenantB); err != nil {
		t.Fatalf(
			"project B must remain after deleting A: %v",
			err,
		)
	}
}
