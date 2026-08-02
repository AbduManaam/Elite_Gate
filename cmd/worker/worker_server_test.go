package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"elitegate/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerMetricsServer_EndpointsAndShutdown(t *testing.T) {
	t.Setenv("WORKER_METRICS_ADDR", "127.0.0.1:9092")

	reg := prometheus.NewRegistry()
	_ = metrics.NewCustomDomainMetrics(reg)
	server := StartWorkerMetricsServer(reg, zerolog.Nop())
	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	// Wait briefly for server to listen
	time.Sleep(50 * time.Millisecond)

	// 1. Test /healthz endpoint
	respHealth, err := http.Get("http://127.0.0.1:9092/healthz")
	require.NoError(t, err)
	defer respHealth.Body.Close()

	assert.Equal(t, http.StatusOK, respHealth.StatusCode)
	bodyHealth, err := io.ReadAll(respHealth.Body)
	require.NoError(t, err)

	var healthMap map[string]string
	err = json.Unmarshal(bodyHealth, &healthMap)
	require.NoError(t, err)
	assert.Equal(t, "worker", healthMap["service"])
	assert.Equal(t, "ok", healthMap["status"])
	assert.Equal(t, "1.0.0", healthMap["version"])

	// 2. Test /metrics endpoint
	respMetrics, err := http.Get("http://127.0.0.1:9092/metrics")
	require.NoError(t, err)
	defer respMetrics.Body.Close()

	assert.Equal(t, http.StatusOK, respMetrics.StatusCode)
	bodyMetrics, err := io.ReadAll(respMetrics.Body)
	require.NoError(t, err)
	assert.Contains(t, string(bodyMetrics), "# HELP")

	// 3. Test Graceful Shutdown
	err = server.Shutdown(context.Background())
	require.NoError(t, err)

	// Verify server stopped accepting connections
	_, err = http.Get("http://127.0.0.1:9092/healthz")
	assert.Error(t, err)
}
