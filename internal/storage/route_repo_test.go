package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type mockRouteDBDriver struct {
	mu     sync.Mutex
	execFn func(query string, args []driver.Value) (driver.Result, error)
}

func (d *mockRouteDBDriver) Open(name string) (driver.Conn, error) {
	return &mockRouteDBConn{d}, nil
}

type mockRouteDBConn struct {
	drv *mockRouteDBDriver
}

func (c *mockRouteDBConn) Prepare(query string) (driver.Stmt, error) {
	return &mockRouteDBStmt{c.drv, query}, nil
}

func (c *mockRouteDBConn) Close() error { return nil }

func (c *mockRouteDBConn) Begin() (driver.Tx, error) {
	return &mockRouteTx{}, nil
}

type mockRouteTx struct{}

func (mockRouteTx) Commit() error   { return nil }
func (mockRouteTx) Rollback() error { return nil }

type mockRouteDBStmt struct {
	drv   *mockRouteDBDriver
	query string
}

func (s *mockRouteDBStmt) Close() error  { return nil }
func (s *mockRouteDBStmt) NumInput() int { return -1 }

func (s *mockRouteDBStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &mockRouteRows{}, nil
}

func (s *mockRouteDBStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.drv.mu.Lock()
	fn := s.drv.execFn
	s.drv.mu.Unlock()
	if fn != nil {
		return fn(s.query, args)
	}
	return &mockRouteResult{1}, nil
}

type mockRouteRows struct{}

func (mockRouteRows) Columns() []string              { return []string{"config"} }
func (mockRouteRows) Close() error                   { return nil }
func (mockRouteRows) Next(dest []driver.Value) error { return io.EOF }

type mockRouteResult struct {
	rowsAffected int64
}

func (r *mockRouteResult) LastInsertId() (int64, error) { return 0, nil }
func (r *mockRouteResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

var (
	routeRegisterOnce sync.Once
	routeSqlDrv       = &mockRouteDBDriver{}
)

func initRouteMockDB(t *testing.T) *sql.DB {
	routeRegisterOnce.Do(func() {
		sql.Register("mock_route_db", routeSqlDrv)
	})
	db, err := sql.Open("mock_route_db", "test")
	if err != nil {
		t.Fatalf("failed to open route mock db: %v", err)
	}
	return db
}

func TestRouteRepo_Enable_Success(t *testing.T) {
	db := initRouteMockDB(t)
	defer db.Close()

	routeSqlDrv.mu.Lock()
	routeSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		return &mockRouteResult{rowsAffected: 1}, nil
	}
	routeSqlDrv.mu.Unlock()

	repo := NewRouteRepo(db, zerolog.Nop())

	projectID := uuid.New()
	userID := uuid.New()
	tc := TenantContext{
		ProjectID: projectID,
		UserID:    userID,
	}
	ctx := WithTenantContext(context.Background(), tc)

	err := repo.Enable(ctx, "route-123")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRouteRepo_Enable_NotFound(t *testing.T) {
	db := initRouteMockDB(t)
	defer db.Close()

	routeSqlDrv.mu.Lock()
	routeSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		// If it's the SELECT for tenant session, return 1 row affected (success),
		// but if it's the UPDATE routes query, return 0 rows affected (NotFound).
		if query != "SELECT\n\t\t\tset_config('app.project_id', $1::text, TRUE),\n\t\t\tset_config('app.current_user_id', $2::text, TRUE)" {
			return &mockRouteResult{rowsAffected: 0}, nil
		}
		return &mockRouteResult{rowsAffected: 1}, nil
	}
	routeSqlDrv.mu.Unlock()

	repo := NewRouteRepo(db, zerolog.Nop())

	projectID := uuid.New()
	userID := uuid.New()
	tc := TenantContext{
		ProjectID: projectID,
		UserID:    userID,
	}
	ctx := WithTenantContext(context.Background(), tc)

	err := repo.Enable(ctx, "route-not-exist")
	if !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("expected ErrRouteNotFound, got: %v", err)
	}
}
