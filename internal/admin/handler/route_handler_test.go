package handler

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"elitegate/internal/storage"
)

type mockRouteHandlerDBDriver struct {
	mu     sync.Mutex
	execFn func(query string, args []driver.Value) (driver.Result, error)
}

func (d *mockRouteHandlerDBDriver) Open(name string) (driver.Conn, error) {
	return &mockRouteHandlerDBConn{d}, nil
}

type mockRouteHandlerDBConn struct {
	drv *mockRouteHandlerDBDriver
}

func (c *mockRouteHandlerDBConn) Prepare(query string) (driver.Stmt, error) {
	return &mockRouteHandlerDBStmt{c.drv, query}, nil
}

func (c *mockRouteHandlerDBConn) Close() error { return nil }

func (c *mockRouteHandlerDBConn) Begin() (driver.Tx, error) {
	return &mockRouteHandlerTx{}, nil
}

type mockRouteHandlerTx struct{}

func (mockRouteHandlerTx) Commit() error   { return nil }
func (mockRouteHandlerTx) Rollback() error { return nil }

type mockRouteHandlerDBStmt struct {
	drv   *mockRouteHandlerDBDriver
	query string
}

func (s *mockRouteHandlerDBStmt) Close() error { return nil }
func (s *mockRouteHandlerDBStmt) NumInput() int { return -1 }

func (s *mockRouteHandlerDBStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &mockRouteHandlerRows{}, nil
}

func (s *mockRouteHandlerDBStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.drv.mu.Lock()
	fn := s.drv.execFn
	s.drv.mu.Unlock()
	if fn != nil {
		return fn(s.query, args)
	}
	return &mockRouteHandlerResult{1}, nil
}

type mockRouteHandlerRows struct{}

func (mockRouteHandlerRows) Columns() []string              { return []string{"config"} }
func (mockRouteHandlerRows) Close() error                   { return nil }
func (mockRouteHandlerRows) Next(dest []driver.Value) error { return io.EOF }

type mockRouteHandlerResult struct {
	rowsAffected int64
}

func (r *mockRouteHandlerResult) LastInsertId() (int64, error) { return 0, nil }
func (r *mockRouteHandlerResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

var (
	routeHandlerRegisterOnce sync.Once
	routeHandlerSqlDrv       = &mockRouteHandlerDBDriver{}
)

func initRouteHandlerMockDB(t *testing.T) *sql.DB {
	routeHandlerRegisterOnce.Do(func() {
		sql.Register("mock_route_handler_db", routeHandlerSqlDrv)
	})
	db, err := sql.Open("mock_route_handler_db", "test")
	if err != nil {
		t.Fatalf("failed to open route handler mock db: %v", err)
	}
	return db
}

func TestRouteHandler_Enable_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := initRouteHandlerMockDB(t)
	defer db.Close()

	routeHandlerSqlDrv.mu.Lock()
	routeHandlerSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		return &mockRouteHandlerResult{rowsAffected: 1}, nil
	}
	routeHandlerSqlDrv.mu.Unlock()

	repo := storage.NewRouteRepo(db, zerolog.Nop())
	h := NewRouteHandler(repo, zerolog.Nop(), nil)

	r := gin.New()
	// Middleware to inject TenantContext to simulate RBAC/Scope passing
	r.Use(func(c *gin.Context) {
		tc := storage.TenantContext{
			ProjectID: uuid.New(),
			UserID:    uuid.New(),
		}
		c.Set("tenant_ctx", tc)
		ctx := storage.WithTenantContext(c.Request.Context(), tc)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.PATCH("/routes/:id/enable", h.Enable)

	req, _ := http.NewRequest("PATCH", "/routes/route-123/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestRouteHandler_Enable_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := initRouteHandlerMockDB(t)
	defer db.Close()

	routeHandlerSqlDrv.mu.Lock()
	routeHandlerSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		// Mock 0 rows affected for update to trigger ErrRouteNotFound
		if query != "SELECT\n\t\t\tset_config('app.project_id', $1::text, TRUE),\n\t\t\tset_config('app.current_user_id', $2::text, TRUE)" {
			return &mockRouteHandlerResult{rowsAffected: 0}, nil
		}
		return &mockRouteHandlerResult{rowsAffected: 1}, nil
	}
	routeHandlerSqlDrv.mu.Unlock()

	repo := storage.NewRouteRepo(db, zerolog.Nop())
	h := NewRouteHandler(repo, zerolog.Nop(), nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		tc := storage.TenantContext{
			ProjectID: uuid.New(),
			UserID:    uuid.New(),
		}
		c.Set("tenant_ctx", tc)
		ctx := storage.WithTenantContext(c.Request.Context(), tc)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.PATCH("/routes/:id/enable", h.Enable)

	req, _ := http.NewRequest("PATCH", "/routes/missing-route/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestRouteHandler_Enable_MissingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := initRouteHandlerMockDB(t)
	defer db.Close()

	repo := storage.NewRouteRepo(db, zerolog.Nop())
	h := NewRouteHandler(repo, zerolog.Nop(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{} // empty params

	h.Enable(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestRouteHandler_Enable_RepoError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := initRouteHandlerMockDB(t)
	defer db.Close()

	routeHandlerSqlDrv.mu.Lock()
	routeHandlerSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		// Mock generic error
		if query != "SELECT\n\t\t\tset_config('app.project_id', $1::text, TRUE),\n\t\t\tset_config('app.current_user_id', $2::text, TRUE)" {
			return nil, errors.New("db error")
		}
		return &mockRouteHandlerResult{rowsAffected: 1}, nil
	}
	routeHandlerSqlDrv.mu.Unlock()

	repo := storage.NewRouteRepo(db, zerolog.Nop())
	h := NewRouteHandler(repo, zerolog.Nop(), nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		tc := storage.TenantContext{
			ProjectID: uuid.New(),
			UserID:    uuid.New(),
		}
		c.Set("tenant_ctx", tc)
		ctx := storage.WithTenantContext(c.Request.Context(), tc)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.PATCH("/routes/:id/enable", h.Enable)

	req, _ := http.NewRequest("PATCH", "/routes/route-123/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d. Body: %s", w.Code, w.Body.String())
	}
}
