package handler

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type mockGatewayHandlerDBDriver struct {
	mu      sync.Mutex
	queries []string
	execFn  func(query string, args []driver.Value) (driver.Result, error)
}

func (d *mockGatewayHandlerDBDriver) Open(name string) (driver.Conn, error) {
	return &mockGatewayHandlerDBConn{d}, nil
}

type mockGatewayHandlerDBConn struct {
	drv *mockGatewayHandlerDBDriver
}

func (c *mockGatewayHandlerDBConn) Prepare(query string) (driver.Stmt, error) {
	return &mockGatewayHandlerDBStmt{c.drv, query}, nil
}

func (c *mockGatewayHandlerDBConn) Close() error { return nil }
func (c *mockGatewayHandlerDBConn) Begin() (driver.Tx, error) {
	return &mockGatewayHandlerTx{}, nil
}

type mockGatewayHandlerTx struct{}

func (mockGatewayHandlerTx) Commit() error   { return nil }
func (mockGatewayHandlerTx) Rollback() error { return nil }

type mockGatewayHandlerDBStmt struct {
	drv   *mockGatewayHandlerDBDriver
	query string
}

func (s *mockGatewayHandlerDBStmt) Close() error  { return nil }
func (s *mockGatewayHandlerDBStmt) NumInput() int { return -1 }

func (s *mockGatewayHandlerDBStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &mockGatewayHandlerRows{}, nil
}

func (s *mockGatewayHandlerDBStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.drv.mu.Lock()
	s.drv.queries = append(s.drv.queries, s.query)
	fn := s.drv.execFn
	s.drv.mu.Unlock()
	if fn != nil {
		return fn(s.query, args)
	}
	return &mockGatewayHandlerResult{1}, nil
}

type mockGatewayHandlerRows struct{}

func (mockGatewayHandlerRows) Columns() []string              { return []string{"id"} }
func (mockGatewayHandlerRows) Close() error                   { return nil }
func (mockGatewayHandlerRows) Next(dest []driver.Value) error { return io.EOF }

type mockGatewayHandlerResult struct {
	rowsAffected int64
}

func (r *mockGatewayHandlerResult) LastInsertId() (int64, error) { return 0, nil }
func (r *mockGatewayHandlerResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

var (
	gatewayHandlerRegisterOnce sync.Once
	gatewayHandlerSqlDrv       = &mockGatewayHandlerDBDriver{}
)

func initGatewayHandlerMockDB(t *testing.T) (*sql.DB, *mockGatewayHandlerDBDriver) {
	gatewayHandlerRegisterOnce.Do(func() {
		sql.Register("mock_gateway_handler_db", gatewayHandlerSqlDrv)
	})
	gatewayHandlerSqlDrv.mu.Lock()
	gatewayHandlerSqlDrv.queries = nil
	gatewayHandlerSqlDrv.execFn = nil
	gatewayHandlerSqlDrv.mu.Unlock()

	db, err := sql.Open("mock_gateway_handler_db", "test")
	if err != nil {
		t.Fatalf("failed to open gateway handler mock db: %v", err)
	}
	return db, gatewayHandlerSqlDrv
}

type mockContainerManager struct {
	provisionFn    func(ctx context.Context, gatewayID, projectID, plan string) (endpointIP, gatewayPort, publicHost, publicPort string, err error)
	decommissionFn func(ctx context.Context, gatewayID string) error
}

func (m *mockContainerManager) Provision(ctx context.Context, gatewayID, projectID, plan string) (string, string, string, string, error) {
	if m.provisionFn != nil {
		return m.provisionFn(ctx, gatewayID, projectID, plan)
	}
	return "172.18.0.2", "8080", "localhost", "49152", nil
}

func (m *mockContainerManager) Decommission(ctx context.Context, gatewayID string) error {
	if m.decommissionFn != nil {
		return m.decommissionFn(ctx, gatewayID)
	}
	return nil
}

func TestGatewayHandler_Provision_Success(t *testing.T) {
	db, mockDrv := initGatewayHandlerMockDB(t)
	defer db.Close()

	repo := storage.NewGatewayRepo(db)
	mockContainer := &mockContainerManager{}
	handler := NewGatewayHandler(repo, mockContainer, 5*time.Second)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/admin/v1/projects/:projectId/gateways", handler.Provision)

	projectID := uuid.NewString()
	body := map[string]string{
		"project_id": projectID,
		"plan":       "developer",
	}
	jsonBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/admin/v1/projects/"+projectID+"/gateways", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected status HTTP 202 Accepted, got %d. Body: %s", w.Code, w.Body.String())
	}

	var res map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}

	if res["status"] != "provisioning" {
		t.Errorf("expected status 'provisioning', got %v", res["status"])
	}
	if res["provisioning_status"] != "container_ready" {
		t.Errorf("expected provisioning_status 'container_ready', got %v", res["provisioning_status"])
	}
	if res["host_port"] != "49152" {
		t.Errorf("expected host_port '49152', got %v", res["host_port"])
	}
	if res["status"] == "active" {
		t.Error("response must not claim gateway is active")
	}

	// Verify absence of active public endpoint claim fields
	if _, exists := res["public_endpoint"]; exists {
		t.Error("response must not claim public_endpoint exists")
	}

	// Verify DB queries executed
	mockDrv.mu.Lock()
	queries := append([]string{}, mockDrv.queries...)
	mockDrv.mu.Unlock()

	markReadyCalled := false
	registerCalled := false

	for _, q := range queries {
		if strings.Contains(q, "provisioning_status = 'container_ready'") {
			markReadyCalled = true
		}
		if strings.Contains(q, "status = 'active'") {
			registerCalled = true
		}
	}

	if !markReadyCalled {
		t.Error("expected MarkContainerReady DB query to be executed")
	}
	if registerCalled {
		t.Error("expected old Register DB query (marking active) NOT to be executed")
	}
}

func TestGatewayHandler_Provision_DockerFailure(t *testing.T) {
	db, mockDrv := initGatewayHandlerMockDB(t)
	defer db.Close()

	repo := storage.NewGatewayRepo(db)
	mockContainer := &mockContainerManager{
		provisionFn: func(ctx context.Context, gatewayID, projectID, plan string) (string, string, string, string, error) {
			return "", "", "", "", errors.New("docker daemon error")
		},
	}
	handler := NewGatewayHandler(repo, mockContainer, 5*time.Second)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/admin/v1/projects/:projectId/gateways", handler.Provision)

	projectID := uuid.NewString()
	body := map[string]string{
		"project_id": projectID,
		"plan":       "developer",
	}
	jsonBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/admin/v1/projects/"+projectID+"/gateways", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status HTTP 500, got %d", w.Code)
	}

	// Verify UpdateStatus failed query executed
	mockDrv.mu.Lock()
	queries := append([]string{}, mockDrv.queries...)
	mockDrv.mu.Unlock()

	updateFailedCalled := false
	for _, q := range queries {
		if strings.Contains(q, "status") && strings.Contains(q, "gateways") {
			updateFailedCalled = true
		}
	}
	if !updateFailedCalled {
		t.Error("expected DB status update to 'failed' on Docker failure")
	}
}

func TestGatewayHandler_Provision_RepoInsertFailure(t *testing.T) {
	db, mockDrv := initGatewayHandlerMockDB(t)
	defer db.Close()

	mockDrv.mu.Lock()
	mockDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		return nil, errors.New("db insert error")
	}
	mockDrv.mu.Unlock()

	repo := storage.NewGatewayRepo(db)
	mockContainer := &mockContainerManager{}
	handler := NewGatewayHandler(repo, mockContainer, 5*time.Second)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/admin/v1/projects/:projectId/gateways", handler.Provision)

	projectID := uuid.NewString()
	body := map[string]string{
		"project_id": projectID,
		"plan":       "developer",
	}
	jsonBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/admin/v1/projects/"+projectID+"/gateways", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status HTTP 500, got %d", w.Code)
	}
}
