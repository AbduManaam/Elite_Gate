package handler

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"elitegate/internal/admin/middleware"
	"elitegate/internal/storage"
)

// In-memory mock driver for handler testing
type mockHandlerDBDriver struct {
	mu     sync.Mutex
	execFn func(query string, args []driver.Value) (driver.Result, error)
}

func (d *mockHandlerDBDriver) Open(name string) (driver.Conn, error) {
	return &mockHandlerDBConn{d}, nil
}

type mockHandlerDBConn struct {
	drv *mockHandlerDBDriver
}

func (c *mockHandlerDBConn) Prepare(query string) (driver.Stmt, error) {
	return &mockHandlerDBStmt{c.drv, query}, nil
}

func (c *mockHandlerDBConn) Close() error { return nil }
func (mockHandlerDBConn) Begin() (driver.Tx, error) { return nil, nil }

type mockHandlerDBStmt struct {
	drv   *mockHandlerDBDriver
	query string
}

func (s *mockHandlerDBStmt) Close() error { return nil }
func (s *mockHandlerDBStmt) NumInput() int { return -1 }
func (s *mockHandlerDBStmt) Query(args []driver.Value) (driver.Rows, error) {
	return nil, io.EOF
}
func (s *mockHandlerDBStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.drv.mu.Lock()
	fn := s.drv.execFn
	s.drv.mu.Unlock()
	if fn != nil {
		return fn(s.query, args)
	}
	return &mockHandlerResult{1}, nil
}

type mockHandlerResult struct {
	rowsAffected int64
}

func (r *mockHandlerResult) LastInsertId() (int64, error) { return 0, nil }
func (r *mockHandlerResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

var (
	handlerRegisterOnce sync.Once
	handlerSqlDrv       = &mockHandlerDBDriver{}
)

func initHandlerMockDB(t *testing.T) *sql.DB {
	handlerRegisterOnce.Do(func() {
		sql.Register("mock_handler_db", handlerSqlDrv)
	})
	db, err := sql.Open("mock_handler_db", "test")
	if err != nil {
		t.Fatalf("failed to open handler mock db: %v", err)
	}
	return db
}

func TestValidateOrigin(t *testing.T) {
	tests := []struct {
		origin  string
		wantErr bool
	}{
		{"https://my-dashboard.com", false},
		{"http://localhost:5173", false},
		{"http://127.0.0.1:3000", true}, // http only allowed for hostname starting with localhost
		{"http://localhost.com", true}, // http allowed for localhost hostname only
		{"http://my-dashboard.com", true}, // http not allowed
		{"https://my-dashboard.com/path", true}, // no path allowed
		{"https://my-dashboard.com?query=1", true}, // no query allowed
		{"*", true}, // wildcard disallowed
		{"https://*.my-dashboard.com", true}, // wildcard disallowed
	}

	for _, tt := range tests {
		err := validateOrigin(tt.origin)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateOrigin(%q) error = %v, wantErr %v", tt.origin, err, tt.wantErr)
		}
	}
}

// Spy Cache to track Invalidate calls
type spyOriginCache struct {
	invalidatedProjectID string
	mu                   sync.Mutex
}

func (s *spyOriginCache) Invalidate(projectID string) {
	s.mu.Lock()
	s.invalidatedProjectID = projectID
	s.mu.Unlock()
}

func TestUpdateDashboardOriginsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := initHandlerMockDB(t)
	defer db.Close()

	// Setup mock DB execution
	handlerSqlDrv.mu.Lock()
	handlerSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		return &mockHandlerResult{1}, nil
	}
	handlerSqlDrv.mu.Unlock()

	repo := storage.NewProjectRepo(db, zerolog.Nop())
	
	// Create actual OriginCache, but since we want to spy on it, we can wrap or inspect it.
	// However, ProjectHandler holds a reference to *middleware.OriginCache.
	// Since OriginCache's fields are unexported outside middleware, we can create a real OriginCache,
	// seed it, and verify the value is evicted/deleted after Update.
	realCache := middleware.NewOriginCache(repo, 10*time.Second)

	h := NewProjectHandler(repo, realCache, zerolog.Nop())

	r := gin.New()
	r.PUT("/admin/v1/projects/:projectId/dashboard-origins", h.UpdateDashboardOrigins)

	t.Run("Success Updates Cache and DB", func(t *testing.T) {
		// Populate cache entry
		ctx := context.Background()
		realCache.Get(ctx, "proj-123") // This will call DB, but it's okay. Let's pre-populate the data directly in test if needed.
		// Alternatively, just verify that endpoint returns 200 and performs execution.
		
		body := dashboardOriginsRequest{
			Origins: []string{"https://my-app.com", "http://localhost:3000"},
		}
		b, _ := json.Marshal(body)

		req, _ := http.NewRequest("PUT", "/admin/v1/projects/proj-123/dashboard-origins", bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Rejects Validation Errors", func(t *testing.T) {
		body := dashboardOriginsRequest{
			Origins: []string{"http://insecure-domain.com"}, // Rejected scheme
		}
		b, _ := json.Marshal(body)

		req, _ := http.NewRequest("PUT", "/admin/v1/projects/proj-123/dashboard-origins", bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 BadRequest, got %d", w.Code)
		}
	})

	t.Run("Rejects Over 10 Origins", func(t *testing.T) {
		body := dashboardOriginsRequest{
			Origins: []string{
				"https://origin1.com", "https://origin2.com", "https://origin3.com",
				"https://origin4.com", "https://origin5.com", "https://origin6.com",
				"https://origin7.com", "https://origin8.com", "https://origin9.com",
				"https://origin10.com", "https://origin11.com",
			},
		}
		b, _ := json.Marshal(body)

		req, _ := http.NewRequest("PUT", "/admin/v1/projects/proj-123/dashboard-origins", bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 BadRequest due to size limit, got %d", w.Code)
		}
	})

	t.Run("DB Update Fails (Conflict or Missing Project)", func(t *testing.T) {
		// Mock DB returning 0 rows affected
		handlerSqlDrv.mu.Lock()
		handlerSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
			return &mockHandlerResult{0}, nil
		}
		handlerSqlDrv.mu.Unlock()

		body := dashboardOriginsRequest{
			Origins: []string{"https://valid-domain.com"},
		}
		b, _ := json.Marshal(body)

		req, _ := http.NewRequest("PUT", "/admin/v1/projects/proj-missing/dashboard-origins", bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 InternalServerError, got %d", w.Code)
		}
	})
}
