package integration

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"

	"elitegate/internal/model"
	"elitegate/internal/storage"
)

func testAdminDSN() string {
	if dsn := os.Getenv("TEST_ADMIN_DATABASE_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://postgres:postgres@127.0.0.1:5433/elitegate_test?sslmode=disable"
}

func testAppDSN() string {
	if dsn := os.Getenv("TEST_APP_DATABASE_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://gate_app:gate_app_password@127.0.0.1:5433/elitegate_test?sslmode=disable"
}

func TestTenantIsolation(t *testing.T) {
	// Connect to development DB as superuser to set up permissions and test data
	dsn := testAdminDSN()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	// Ensure the non-superuser role 'gate_app' exists with a password and has permissions on all tables.
	// In PostgreSQL, superusers always bypass Row-Level Security (even with FORCE RLS),
	// so we must run our test queries using a non-superuser connection.
	_, err = db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'gate_app') THEN
				CREATE ROLE gate_app WITH LOGIN PASSWORD 'gate_app_password';
			END IF;
		END
		$$;
		ALTER ROLE gate_app WITH PASSWORD 'gate_app_password';
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO gate_app;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO gate_app;
	`)
	if err != nil {
		t.Fatalf("Failed to setup gate_app role permissions: %v", err)
	}

	ctx := context.Background()
	logger := zerolog.Nop()

	// 1. Create two test admin users
	userA := uuid.New()
	userB := uuid.New()
	_, err = db.ExecContext(ctx, `
		INSERT INTO admin_users (id, username, password_hash, email) VALUES
		($1, 'test_user_a', 'hash', 'test_user_a@elitegate.local'),
		($2, 'test_user_b', 'hash', 'test_user_b@elitegate.local')
	`, userA, userB)
	if err != nil {
		t.Fatalf("Failed to insert admin users: %v", err)
	}
	defer func() {
		db.ExecContext(ctx, "DELETE FROM admin_users WHERE id IN ($1, $2)", userA, userB)
	}()

	// 2. Create two test projects
	projectA := uuid.New()
	projectB := uuid.New()
	_, err = db.ExecContext(ctx, `
		INSERT INTO projects (id, name, slug, owner_id) VALUES
		($1, 'Project Alpha', 'test-alpha', $3),
		($2, 'Project Beta', 'test-beta', $4)
	`, projectA, projectB, userA, userB)
	if err != nil {
		t.Fatalf("Failed to insert projects: %v", err)
	}
	defer func() {
		db.ExecContext(ctx, "DELETE FROM projects WHERE id IN ($1, $2)", projectA, projectB)
	}()

	// 3. Create memberships
	_, err = db.ExecContext(ctx, `
		INSERT INTO project_members (project_id, admin_user_id, role) VALUES
		($1, $2, 'owner'),
		($3, $4, 'owner')
	`, projectA, userA, projectB, userB)
	if err != nil {
		t.Fatalf("Failed to insert memberships: %v", err)
	}
	defer func() {
		db.ExecContext(ctx, "DELETE FROM project_members WHERE project_id IN ($1, $2)", projectA, projectB)
	}()

	// Connect as non-superuser for testing RLS operations
	testDSN := testAppDSN()
	testDB, err := sql.Open("postgres", testDSN)
	if err != nil {
		t.Fatalf("Failed to open gate_app database connection: %v", err)
	}
	defer testDB.Close()

	// Initialize repositories with the non-superuser connection
	routeRepo := storage.NewRouteRepo(testDB, logger)

	// Define two TenantContexts
	tenantCtxA := storage.WithTenantContext(ctx, storage.TenantContext{
		ProjectID: projectA,
		UserID:    userA,
		UserRole:  "owner",
	})
	tenantCtxB := storage.WithTenantContext(ctx, storage.TenantContext{
		ProjectID: projectB,
		UserID:    userB,
		UserRole:  "owner",
	})

	// 4. Create a route under Project A
	routeA := &model.Route{
		Name:      "route-alpha",
		Path:      "/alpha-route",
		MatchType: "prefix",
		Enabled:   true,
		Methods:   []string{"GET"},
	}
	err = routeRepo.Create(tenantCtxA, routeA)
	if err != nil {
		t.Fatalf("Failed to create route: %v", err)
	}
	defer func() {
		_ = routeRepo.Delete(tenantCtxA, routeA.ID)
	}()

	// 5. Query the routes using TenantContext B (should NOT return routeA)
	routesB, _, err := routeRepo.ListAll(tenantCtxB, 0, 0)
	if err != nil {
		t.Fatalf("ListAll with tenant ctx B failed: %v", err)
	}
	for _, r := range routesB {
		if r.ID == routeA.ID {
			t.Errorf("Project B should not see Project A's route, but it was returned in ListAll")
		}
	}

	// 6. Query the routes using TenantContext A (should return routeA)
	routesA, _, err := routeRepo.ListAll(tenantCtxA, 0, 0)
	if err != nil {
		t.Fatalf("ListAll with tenant ctx A failed: %v", err)
	}
	found := false
	for _, r := range routesA {
		if r.ID == routeA.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Project A should see its own route, but it was not returned in ListAll")
	}

	// 7. Test Gateway Isolation
	gatewayRepo := storage.NewGatewayRepo(testDB)

	// Provision a gateway for Project A using superuser db connection to bypass RLS setup restriction.
	_, err = db.ExecContext(ctx, `
		INSERT INTO gateways (external_id, project_id, endpoint_ip, gateway_port, plan, status)
		VALUES ('gw_test_alpha_rls', $1, '0.0.0.0', '0', 'shared', 'provisioning')
	`, projectA)
	if err != nil {
		t.Fatalf("Failed to provision gateway: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM gateways WHERE external_id = 'gw_test_alpha_rls'")
	}()

	// Query gateways using TenantContext B (should NOT return gw_test_alpha_rls)
	gatewaysB, _, err := gatewayRepo.ListByProject(tenantCtxB, projectB.String(), 0, 0)
	if err != nil {
		t.Fatalf("ListByProject with tenant ctx B failed: %v", err)
	}
	for _, gw := range gatewaysB {
		if gw.ExternalID == "gw_test_alpha_rls" {
			t.Errorf("Project B should not see Project A's gateway, but it was returned in ListByProject")
		}
	}

	// Query gateways using TenantContext A (should return gw_test_alpha_rls)
	gatewaysA, _, err := gatewayRepo.ListByProject(tenantCtxA, projectA.String(), 0, 0)
	if err != nil {
		t.Fatalf("ListByProject with tenant ctx A failed: %v", err)
	}
	foundGw := false
	for _, gw := range gatewaysA {
		if gw.ExternalID == "gw_test_alpha_rls" {
			foundGw = true
			break
		}
	}
	if !foundGw {
		t.Errorf("Project A should see its own gateway, but it was not returned in ListByProject")
	}
}

func TestGatewayListAllForAdmin(t *testing.T) {
	// Connect to development DB as superuser
	dsn := testAdminDSN()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. Create two test admin users
	userA := uuid.New()
	userB := uuid.New()
	_, err = db.ExecContext(ctx, `
		INSERT INTO admin_users (id, username, password_hash, email) VALUES
		($1, 'test_user_a_all', 'hash', 'test_user_a_all@elitegate.local'),
		($2, 'test_user_b_all', 'hash', 'test_user_b_all@elitegate.local')
	`, userA, userB)
	if err != nil {
		t.Fatalf("Failed to insert admin users: %v", err)
	}
	defer func() {
		db.ExecContext(ctx, "DELETE FROM admin_users WHERE id IN ($1, $2)", userA, userB)
	}()

	// 2. Create two test projects
	projectA := uuid.New()
	projectB := uuid.New()
	_, err = db.ExecContext(ctx, `
		INSERT INTO projects (id, name, slug, owner_id) VALUES
		($1, 'Project Alpha All', 'test-alpha-all', $3),
		($2, 'Project Beta All', 'test-beta-all', $4)
	`, projectA, projectB, userA, userB)
	if err != nil {
		t.Fatalf("Failed to insert projects: %v", err)
	}
	defer func() {
		db.ExecContext(ctx, "DELETE FROM projects WHERE id IN ($1, $2)", projectA, projectB)
	}()

	// 3. Create memberships
	_, err = db.ExecContext(ctx, `
		INSERT INTO project_members (project_id, admin_user_id, role) VALUES
		($1, $2, 'owner'),
		($3, $4, 'owner')
	`, projectA, userA, projectB, userB)
	if err != nil {
		t.Fatalf("Failed to insert memberships: %v", err)
	}
	defer func() {
		db.ExecContext(ctx, "DELETE FROM project_members WHERE project_id IN ($1, $2)", projectA, projectB)
	}()

	// 4. Create gateways for Project A and Project B (active status)
	_, err = db.ExecContext(ctx, `
		INSERT INTO gateways (external_id, project_id, endpoint_ip, gateway_port, plan, status) VALUES
		('gw_test_alpha_all', $1, '127.0.0.1', '8080', 'dedicated', 'active'),
		('gw_test_beta_all', $2, '127.0.0.1', '8080', 'dedicated', 'active')
	`, projectA, projectB)
	if err != nil {
		t.Fatalf("Failed to insert gateways: %v", err)
	}
	defer func() {
		db.ExecContext(ctx, "DELETE FROM gateways WHERE external_id IN ('gw_test_alpha_all', 'gw_test_beta_all')")
	}()

	gatewayRepo := storage.NewGatewayRepo(db)

	// Query gateways for Admin A (should return gw_test_alpha_all, NOT gw_test_beta_all)
	gatewaysA, _, err := gatewayRepo.ListAllForAdmin(ctx, userA.String(), 0, 0)
	if err != nil {
		t.Fatalf("ListAllForAdmin for user A failed: %v", err)
	}

	foundA := false
	for _, gw := range gatewaysA {
		if gw.ExternalID == "gw_test_beta_all" {
			t.Errorf("Admin A should not see Admin B's gateway 'gw_test_beta_all'")
		}
		if gw.ExternalID == "gw_test_alpha_all" {
			foundA = true
		}
	}
	if !foundA {
		t.Errorf("Admin A should see their own gateway 'gw_test_alpha_all'")
	}

	// Query gateways for Admin B (should return gw_test_beta_all, NOT gw_test_alpha_all)
	gatewaysB, _, err := gatewayRepo.ListAllForAdmin(ctx, userB.String(), 0, 0)
	if err != nil {
		t.Fatalf("ListAllForAdmin for user B failed: %v", err)
	}

	foundB := false
	for _, gw := range gatewaysB {
		if gw.ExternalID == "gw_test_alpha_all" {
			t.Errorf("Admin B should not see Admin A's gateway 'gw_test_alpha_all'")
		}
		if gw.ExternalID == "gw_test_beta_all" {
			foundB = true
		}
	}
	if !foundB {
		t.Errorf("Admin B should see their own gateway 'gw_test_beta_all'")
	}
}
