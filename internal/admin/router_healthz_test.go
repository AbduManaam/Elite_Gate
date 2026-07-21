package admin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"elitegate/internal/admin"
	"elitegate/internal/config"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestHealthzEndpoint(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			AllowedOrigins:  []string{"http://localhost:5173"},
			ReadTimeout:     "15s",
			WriteTimeout:    "30s",
			IdleTimeout:     "60s",
			ShutdownTimeout: "30s",
			DrainTimeout:    "5s",
			DrainStaleAfter: "2m",
			MetricsCacheTTL: "10s",
		},
		Auth: config.AuthConfig{
			JWTSecret: "supersecretjwtkey_32byteslongkey!",
		},
	}

	router, err := admin.NewRouter(zerolog.Nop(), nil, cfg, nil)
	assert.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"ok"`)
	assert.Contains(t, w.Body.String(), `"service":"admin-api"`)
}
