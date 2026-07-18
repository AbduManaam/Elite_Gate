package middleware

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"elitegate/internal/storage"
)

// Minimal custom sql.Driver to mock DB queries for testing without running a real DB.
type mockCORSDBDriver struct {
	mu      sync.Mutex
	queryFn func(query string, args []driver.Value) (driver.Rows, error)
}

func (d *mockCORSDBDriver) Open(name string) (driver.Conn, error) {
	return &mockCORSDBConn{d}, nil
}

type mockCORSDBConn struct {
	drv *mockCORSDBDriver
}

func (c *mockCORSDBConn) Prepare(query string) (driver.Stmt, error) {
	return &mockCORSDBStmt{c.drv, query}, nil
}

func (c *mockCORSDBConn) Close() error           { return nil }
func (mockCORSDBConn) Begin() (driver.Tx, error) { return nil, nil }

type mockCORSDBStmt struct {
	drv   *mockCORSDBDriver
	query string
}

func (s *mockCORSDBStmt) Close() error  { return nil }
func (s *mockCORSDBStmt) NumInput() int { return -1 }
func (s *mockCORSDBStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.drv.mu.Lock()
	fn := s.drv.queryFn
	s.drv.mu.Unlock()
	if fn != nil {
		return fn(s.query, args)
	}
	return nil, io.EOF
}
func (s *mockCORSDBStmt) Exec(args []driver.Value) (driver.Result, error) {
	return nil, nil
}

type mockCORSDBRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *mockCORSDBRows) Columns() []string { return r.columns }
func (r *mockCORSDBRows) Close() error      { return nil }
func (r *mockCORSDBRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	for i, v := range r.rows[r.index] {
		dest[i] = v
	}
	r.index++
	return nil
}

var (
	registerOnce sync.Once
	sqlDrv       = &mockCORSDBDriver{}
)

func initMockDB(t *testing.T) *sql.DB {
	registerOnce.Do(func() {
		sql.Register("mock_cors_db", sqlDrv)
	})
	db, err := sql.Open("mock_cors_db", "test")
	if err != nil {
		t.Fatalf("failed to open mock db: %v", err)
	}
	return db
}

func TestDynamicCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := initMockDB(t)
	defer db.Close()

	logger := zerolog.Nop()
	projectRepo := storage.NewProjectRepo(db, logger)
	originCache := NewOriginCache(projectRepo, 10*time.Second)

	staticOrigins := []string{"http://localhost:5173", "https://admin.elitegate.local"}

	// Set up database mock function
	var queryCount int
	var queryMu sync.Mutex
	sqlDrv.mu.Lock()
	sqlDrv.queryFn = func(query string, args []driver.Value) (driver.Rows, error) {
		queryMu.Lock()
		queryCount++
		queryMu.Unlock()

		projectID := args[0].(string)
		var responseStr string
		if projectID == "proj-alpha" {
			responseStr = `{"https://alpha-dash.com"}`
		} else if projectID == "proj-beta" {
			responseStr = `{"https://beta-dash.com"}`
		} else {
			responseStr = `{}`
		}

		return &mockCORSDBRows{
			columns: []string{"dashboard_allowed_origins"},
			rows: [][]driver.Value{
				{responseStr},
			},
		}, nil
	}
	sqlDrv.mu.Unlock()

	// 1. Setup Gin router with DynamicCORS
	r := gin.New()
	r.Use(DynamicCORS(staticOrigins, originCache))
	r.Any("/admin/login", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.Any("/admin/v1/projects/:projectId/summary", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Helper to send requests
	sendReq := func(method, path, origin string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(method, path, nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// Test Case A: Static origin allowed on route with no project context
	t.Run("Static Origin Allowed on Public Route", func(t *testing.T) {
		w := sendReq("GET", "/admin/login", "http://localhost:5173")
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
			t.Errorf("expected origin header to be set, got %q", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	// Test Case B: Disallowed static origin on public route
	t.Run("Disallowed Static Origin on Public Route", func(t *testing.T) {
		w := sendReq("GET", "/admin/login", "http://malicious.com")
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("expected no origin header, got %q", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	// Test Case C: Project-scoped CORS checks (tenant isolation)
	t.Run("Project-scoped CORS Isolation", func(t *testing.T) {
		// Reset count
		queryMu.Lock()
		queryCount = 0
		queryMu.Unlock()

		// Request for Project Alpha with Alpha Origin -> Should succeed
		w1 := sendReq("GET", "/admin/v1/projects/proj-alpha/summary", "https://alpha-dash.com")
		if w1.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w1.Code)
		}
		if w1.Header().Get("Access-Control-Allow-Origin") != "https://alpha-dash.com" {
			t.Errorf("expected allowed origin, got %q", w1.Header().Get("Access-Control-Allow-Origin"))
		}

		// Request for Project Alpha with Beta Origin -> Should be rejected (no CORS header set, continue GET route)
		w2 := sendReq("GET", "/admin/v1/projects/proj-alpha/summary", "https://beta-dash.com")
		if w2.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("expected no origin header, got %q", w2.Header().Get("Access-Control-Allow-Origin"))
		}

		// Request for Project Beta with Beta Origin -> Should succeed
		w3 := sendReq("GET", "/admin/v1/projects/proj-beta/summary", "https://beta-dash.com")
		if w3.Header().Get("Access-Control-Allow-Origin") != "https://beta-dash.com" {
			t.Errorf("expected allowed origin, got %q", w3.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	// Test Case D: Caching verification (Cache hit does not re-query DB)
	t.Run("Cache Hit Optimization", func(t *testing.T) {
		originCache.Invalidate("proj-alpha")

		queryMu.Lock()
		queryCount = 0
		queryMu.Unlock()

		// First hit -> queries database
		sendReq("GET", "/admin/v1/projects/proj-alpha/summary", "https://alpha-dash.com")
		// Second hit -> served from cache
		sendReq("GET", "/admin/v1/projects/proj-alpha/summary", "https://alpha-dash.com")
		// Third hit -> served from cache
		sendReq("GET", "/admin/v1/projects/proj-alpha/summary", "https://alpha-dash.com")

		queryMu.Lock()
		count := queryCount
		queryMu.Unlock()
		if count != 1 {
			t.Errorf("expected exactly 1 database query (cache hits should bypass DB), got %d", count)
		}
	})

	// Test Case E: Invalidation forces a fresh query
	t.Run("Cache Invalidation", func(t *testing.T) {
		originCache.Invalidate("proj-alpha")

		queryMu.Lock()
		queryCount = 0
		queryMu.Unlock()

		// Warm cache
		sendReq("GET", "/admin/v1/projects/proj-alpha/summary", "https://alpha-dash.com")

		// Invalidate
		originCache.Invalidate("proj-alpha")

		// Query again -> should trigger a new DB read
		sendReq("GET", "/admin/v1/projects/proj-alpha/summary", "https://alpha-dash.com")

		queryMu.Lock()
		count := queryCount
		queryMu.Unlock()
		if count != 2 {
			t.Errorf("expected exactly 2 database queries after invalidation, got %d", count)
		}
	})

	// Test Case F: Preflight OPTIONS request checks
	t.Run("Preflight OPTIONS Handling", func(t *testing.T) {
		// Valid preflight
		w1 := sendReq("OPTIONS", "/admin/v1/projects/proj-alpha/summary", "https://alpha-dash.com")
		if w1.Code != http.StatusNoContent {
			t.Errorf("expected 204 NoContent, got %d", w1.Code)
		}

		// Invalid preflight
		w2 := sendReq("OPTIONS", "/admin/v1/projects/proj-alpha/summary", "https://malicious.com")
		if w2.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", w2.Code)
		}
	})
}
