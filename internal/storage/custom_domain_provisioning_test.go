package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"elitegate/internal/domain"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvisioningDBDriver struct {
	mu      sync.Mutex
	queryFn func(query string, args []driver.Value) (driver.Rows, error)
	execFn  func(query string, args []driver.Value) (driver.Result, error)
}

func (d *mockProvisioningDBDriver) Open(name string) (driver.Conn, error) {
	return &mockProvisioningDBConn{d}, nil
}

type mockProvisioningDBConn struct {
	drv *mockProvisioningDBDriver
}

func (c *mockProvisioningDBConn) Prepare(query string) (driver.Stmt, error) {
	return &mockProvisioningDBStmt{c.drv, query}, nil
}

func (c *mockProvisioningDBConn) Close() error { return nil }

func (c *mockProvisioningDBConn) Begin() (driver.Tx, error) {
	return &mockProvisioningTx{}, nil
}

type mockProvisioningTx struct{}

func (mockProvisioningTx) Commit() error   { return nil }
func (mockProvisioningTx) Rollback() error { return nil }

type mockProvisioningDBStmt struct {
	drv   *mockProvisioningDBDriver
	query string
}

func (s *mockProvisioningDBStmt) Close() error  { return nil }
func (s *mockProvisioningDBStmt) NumInput() int { return -1 }

func (s *mockProvisioningDBStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.drv.mu.Lock()
	fn := s.drv.queryFn
	s.drv.mu.Unlock()
	if fn != nil {
		return fn(s.query, args)
	}
	return &mockProvisioningRows{}, nil
}

func (s *mockProvisioningDBStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.drv.mu.Lock()
	fn := s.drv.execFn
	s.drv.mu.Unlock()
	if fn != nil {
		return fn(s.query, args)
	}
	return &mockProvisioningResult{rowsAffected: 1}, nil
}

type mockProvisioningRows struct {
	cols []string
	rows [][]driver.Value
	pos  int
}

func (r *mockProvisioningRows) Columns() []string {
	if len(r.cols) > 0 {
		return r.cols
	}
	if len(r.rows) > 0 {
		cols := make([]string, len(r.rows[0]))
		for i := range cols {
			cols[i] = "col"
		}
		return cols
	}
	return []string{"col"}
}

func (r *mockProvisioningRows) Close() error { return nil }

func (r *mockProvisioningRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.pos]
	for i, val := range row {
		if i < len(dest) {
			dest[i] = val
		}
	}
	r.pos++
	return nil
}

type mockProvisioningResult struct {
	rowsAffected int64
}

func (r *mockProvisioningResult) LastInsertId() (int64, error) { return 0, nil }
func (r *mockProvisioningResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

var (
	provisioningRegisterOnce sync.Once
	provisioningSqlDrv       = &mockProvisioningDBDriver{}
)

func initProvisioningMockDB(t *testing.T) *sql.DB {
	provisioningRegisterOnce.Do(func() {
		sql.Register("mock_provisioning_db", provisioningSqlDrv)
	})
	db, err := sql.Open("mock_provisioning_db", "test")
	require.NoError(t, err)
	return db
}

func TestClaimNextProvisioningJob_Success(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	id := uuid.New()
	projectID := uuid.New()
	leaseToken := uuid.New()
	now := time.Now().UTC()

	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.queryFn = func(query string, args []driver.Value) (driver.Rows, error) {
		return &mockProvisioningRows{
			rows: [][]driver.Value{
				{
					id.String(), projectID.String(), "example.com", "verified", "ready",
					domain.ProvisioningStatusRequestingCertificate, nil, nil,
					true, nil, nil, 0, now, now, now, "worker-1", leaseToken.String(), nil,
				},
			},
		}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	job, err := repo.ClaimNextProvisioningJob(context.Background(), "worker-1", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, id, job.ID)
	assert.Equal(t, "example.com", job.Hostname)
	assert.Equal(t, domain.ProvisioningStatusRequestingCertificate, job.ProvisioningStatus)
}

func TestClaimNextProvisioningJob_NoJob(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.queryFn = func(query string, args []driver.Value) (driver.Rows, error) {
		return &mockProvisioningRows{rows: [][]driver.Value{}}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	job, err := repo.ClaimNextProvisioningJob(context.Background(), "worker-1", 5*time.Minute)
	require.NoError(t, err)
	assert.Nil(t, job)
}

func TestAdvanceProvisioningState_Success(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		return &mockProvisioningResult{rowsAffected: 1}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	err := repo.AdvanceProvisioningState(context.Background(), AdvanceProvisioningParams{
		ID:             uuid.New(),
		LeaseToken:     uuid.New(),
		ExpectedStatus: domain.ProvisioningStatusRequestingCertificate,
		NewStatus:      domain.ProvisioningStatusWaitingForValidationRecord,
	})
	require.NoError(t, err)
}

func TestAdvanceProvisioningState_StaleLease(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		return &mockProvisioningResult{rowsAffected: 0}, nil
	}
	provisioningSqlDrv.queryFn = func(query string, args []driver.Value) (driver.Rows, error) {
		diffLease := uuid.New()
		return &mockProvisioningRows{
			rows: [][]driver.Value{
				{diffLease.String(), domain.ProvisioningStatusRequestingCertificate, nil},
			},
		}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	err := repo.AdvanceProvisioningState(context.Background(), AdvanceProvisioningParams{
		ID:             uuid.New(),
		LeaseToken:     uuid.New(),
		ExpectedStatus: domain.ProvisioningStatusRequestingCertificate,
		NewStatus:      domain.ProvisioningStatusWaitingForValidationRecord,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrStaleLease))
}

func TestScheduleProvisioningPoll_DoesNotIncrementAttempts(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	var executedQuery string
	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		executedQuery = query
		return &mockProvisioningResult{rowsAffected: 1}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	err := repo.ScheduleProvisioningPoll(
		context.Background(),
		uuid.New(),
		uuid.New(),
		domain.ProvisioningStatusWaitingForCertificate,
		time.Now().Add(1*time.Minute),
	)
	require.NoError(t, err)
	assert.NotContains(t, executedQuery, "provisioning_attempts = provisioning_attempts + 1")
}

func TestScheduleProvisioningRetry_IncrementsAttempts(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	var executedQuery string
	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		executedQuery = query
		return &mockProvisioningResult{rowsAffected: 1}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	err := repo.ScheduleProvisioningRetry(
		context.Background(),
		uuid.New(),
		uuid.New(),
		domain.ProvisioningStatusWaitingForCertificate,
		time.Now().Add(5*time.Minute),
		"transient error",
	)
	require.NoError(t, err)
	assert.Contains(t, executedQuery, "provisioning_attempts = provisioning_attempts + 1")
}

func TestMarkProvisioningFailed_TerminalFailure(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		return &mockProvisioningResult{rowsAffected: 1}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	err := repo.MarkProvisioningFailed(
		context.Background(),
		uuid.New(),
		uuid.New(),
		domain.ProvisioningStatusRequestingCertificate,
		"terminal failure error",
	)
	require.NoError(t, err)
}

func TestMarkProvisioningCompleted_Success(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		return &mockProvisioningResult{rowsAffected: 1}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	err := repo.MarkProvisioningCompleted(
		context.Background(),
		uuid.New(),
		uuid.New(),
	)
	require.NoError(t, err)
}

func TestMarkDeprovisioned_Success(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		return &mockProvisioningResult{rowsAffected: 1}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	err := repo.MarkDeprovisioned(
		context.Background(),
		uuid.New(),
		uuid.New(),
	)
	require.NoError(t, err)
}

func TestReleaseProvisioningLease_Success(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.execFn = func(query string, args []driver.Value) (driver.Result, error) {
		return &mockProvisioningResult{rowsAffected: 1}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	err := repo.ReleaseProvisioningLease(
		context.Background(),
		uuid.New(),
		uuid.New(),
	)
	require.NoError(t, err)
}

type mockCustomDomainRows struct {
	rows [][]driver.Value
	pos  int
}

func (r *mockCustomDomainRows) Columns() []string {
	return []string{
		"id", "project_id", "hostname", "status", "verification_token_hash",
		"verification_record_name", "certificate_arn", "certificate_status",
		"failure_reason", "verified_at", "activated_at", "last_checked_at",
		"created_at", "updated_at", "deleted_at", "routing_target",
		"routing_status", "routing_checked_at", "routing_error",
		"certificate_managed_by_elitegate", "provisioning_status",
		"certificate_validation_name", "certificate_validation_value",
		"certificate_requested_at", "certificate_issued_at",
		"certificate_attached_at", "provisioning_started_at",
		"provisioning_completed_at", "deprovisioning_started_at",
		"provisioning_error", "provisioning_attempts", "next_retry_at",
		"locked_at", "locked_by", "lease_token",
	}
}

func (r *mockCustomDomainRows) Close() error { return nil }

func (r *mockCustomDomainRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.pos]
	for i, val := range row {
		if i < len(dest) {
			dest[i] = val
		}
	}
	r.pos++
	return nil
}

func TestEnqueueProvisioning_Success(t *testing.T) {
	db := initProvisioningMockDB(t)
	defer db.Close()

	id := uuid.New()
	projectID := uuid.New()
	now := time.Now().UTC()

	provisioningSqlDrv.mu.Lock()
	provisioningSqlDrv.queryFn = func(query string, args []driver.Value) (driver.Rows, error) {
		return &mockCustomDomainRows{
			rows: [][]driver.Value{
				{
					id.String(), projectID.String(), "example.com", "verified", "hash123",
					"_verify.example.com", nil, nil, nil, now, nil, now,
					now, now, nil, "target.acm", "ready", now, nil,
					true, domain.ProvisioningStatusRequestingCertificate, nil, nil,
					now, nil, nil, now, nil, nil,
					nil, 0, now, nil, nil, nil,
				},
			},
		}, nil
	}
	provisioningSqlDrv.mu.Unlock()

	repo := NewCustomDomainRepo(db, zerolog.Nop())
	cd, err := repo.EnqueueProvisioning(context.Background(), id, projectID)
	require.NoError(t, err)
	require.NotNil(t, cd)
	assert.Equal(t, domain.ProvisioningStatusRequestingCertificate, cd.ProvisioningStatus)
}
