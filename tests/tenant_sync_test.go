package tests

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"elitegate/internal/admin/handler"
)

type mockTenantSyncDBDriver struct {
	mu     sync.Mutex
	execFn func(query string, args []driver.Value) (driver.Result, error)
}

func (d *mockTenantSyncDBDriver) Open(name string) (driver.Conn, error) {
	return &mockTenantSyncDBConn{d}, nil
}

type mockTenantSyncDBConn struct {
	drv *mockTenantSyncDBDriver
}

func (c *mockTenantSyncDBConn) Prepare(query string) (driver.Stmt, error) {
	return &mockTenantSyncDBStmt{c.drv, query}, nil
}

func (c *mockTenantSyncDBConn) Close() error { return nil }

func (c *mockTenantSyncDBConn) Begin() (driver.Tx, error) {
	return &mockTenantSyncTx{}, nil
}

type mockTenantSyncTx struct{}

func (mockTenantSyncTx) Commit() error   { return nil }
func (mockTenantSyncTx) Rollback() error { return nil }

type mockTenantSyncDBStmt struct {
	drv   *mockTenantSyncDBDriver
	query string
}

func (s *mockTenantSyncDBStmt) Close() error  { return nil }
func (s *mockTenantSyncDBStmt) NumInput() int { return -1 }

func (s *mockTenantSyncDBStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &mockTenantSyncRows{}, nil
}

func (s *mockTenantSyncDBStmt) Exec(args []driver.Value) (driver.Result, error) {
	return &mockTenantSyncResult{1}, nil
}

type mockTenantSyncRows struct{}

func (mockTenantSyncRows) Columns() []string              { return []string{"id"} }
func (mockTenantSyncRows) Close() error                   { return nil }
func (mockTenantSyncRows) Next(dest []driver.Value) error { return io.EOF }

type mockTenantSyncResult struct {
	rowsAffected int64
}

func (r *mockTenantSyncResult) LastInsertId() (int64, error) { return 0, nil }
func (r *mockTenantSyncResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

var (
	tenantSyncRegisterOnce sync.Once
	tenantSyncSqlDrv       = &mockTenantSyncDBDriver{}
)

func initTenantSyncMockDB(t *testing.T) *sql.DB {
	tenantSyncRegisterOnce.Do(func() {
		sql.Register("mock_tenant_sync_db", tenantSyncSqlDrv)
	})
	db, err := sql.Open("mock_tenant_sync_db", "test")
	if err != nil {
		t.Fatalf("failed to open tenant sync mock db: %v", err)
	}
	return db
}

func TestTenantSyncHandler_GetTenantSnapshot_InvalidProjectID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := initTenantSyncMockDB(t)
	defer db.Close()

	h := handler.NewTenantSyncHandler(db, zerolog.Nop())

	r := gin.New()
	r.GET("/projects/:project_id/sync", h.GetTenantSnapshot)

	req, _ := http.NewRequest(http.MethodGet, "/projects/invalid-uuid/sync", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 bad request, got %d", w.Code)
	}
}
