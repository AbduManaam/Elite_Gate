package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProjectJWTHandlerService struct {
	cfg *model.ProjectJWTConfig

	getErr       error
	configureErr error
	deleteErr    error

	configureInput service.ProjectJWTConfigInput

	getCalls       int
	configureCalls int
	deleteCalls    int
}

func (f *fakeProjectJWTHandlerService) Get(
	_ context.Context,
) (*model.ProjectJWTConfig, error) {
	f.getCalls++

	if f.getErr != nil {
		return nil, f.getErr
	}

	return f.cfg, nil
}

func (f *fakeProjectJWTHandlerService) Configure(
	_ context.Context,
	input service.ProjectJWTConfigInput,
) (*model.ProjectJWTConfig, error) {
	f.configureCalls++
	f.configureInput = input

	if f.configureErr != nil {
		return nil, f.configureErr
	}

	return f.cfg, nil
}

func (f *fakeProjectJWTHandlerService) Delete(
	_ context.Context,
) error {
	f.deleteCalls++

	return f.deleteErr
}

func newProjectJWTHandlerTestRouter(
	svc ProjectJWTConfigService,
) *gin.Engine {
	gin.SetMode(gin.TestMode)

	h := NewProjectJWTConfigHandler(
		svc,
		zerolog.Nop(),
		nil,
	)

	r := gin.New()

	r.GET(
		"/projects/:projectId/security/jwt",
		h.Get,
	)

	r.PUT(
		"/projects/:projectId/security/jwt",
		h.Configure,
	)

	r.DELETE(
		"/projects/:projectId/security/jwt",
		h.Delete,
	)

	return r
}

func TestProjectJWTConfigHandler_GetNotConfigured(
	t *testing.T,
) {
	svc := &fakeProjectJWTHandlerService{
		getErr: storage.ErrProjectJWTConfigNotFound,
	}

	router :=
		newProjectJWTHandlerTestRouter(svc)

	req := httptest.NewRequest(
		http.MethodGet,
		"/projects/project-a/security/jwt",
		nil,
	)

	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(
		t,
		http.StatusOK,
		w.Code,
	)

	var response map[string]any

	require.NoError(
		t,
		json.Unmarshal(
			w.Body.Bytes(),
			&response,
		),
	)

	assert.Equal(
		t,
		false,
		response["configured"],
	)

	assert.Equal(
		t,
		false,
		response["secret_configured"],
	)

	assert.Equal(
		t,
		"HS256",
		response["algorithm"],
	)

	// Internal secret information must never appear.
	assert.NotContains(
		t,
		w.Body.String(),
		"secret_arn",
	)

	assert.NotContains(
		t,
		w.Body.String(),
		"secret_version_id",
	)
}

func TestProjectJWTConfigHandler_ConfigureDoesNotLeakSecret(
	t *testing.T,
) {
	svc := &fakeProjectJWTHandlerService{
		cfg: &model.ProjectJWTConfig{
			Enabled:   true,
			Algorithm: "HS256",

			SecretARN: "arn:aws:secretsmanager:test:secret",

			SecretVersionID: "version-1",

			ConfigVersion: 1,

			Audiences: []string{
				"yumzy-api",
			},

			SubjectClaim: "sub",
			RoleClaim:    "role",
			ScopesClaim:  "scope",

			ClockSkewSeconds: 30,
		},
	}

	router :=
		newProjectJWTHandlerTestRouter(svc)

	rawSecret :=
		"12345678901234567890123456789012"

	body := map[string]any{
		"enabled": true,

		"algorithm": "HS256",

		"secret": rawSecret,

		"audiences": []string{"yumzy-api"},

		"subject_claim": "sub",

		"role_claim": "role",

		"scopes_claim": "scope",

		"clock_skew_seconds": 30,
	}

	payload, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPut,
		"/projects/project-a/security/jwt",
		bytes.NewReader(payload),
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(
		t,
		http.StatusOK,
		w.Code,
	)

	assert.Equal(
		t,
		1,
		svc.configureCalls,
	)

	// Service receives the secret because AWS needs it.
	assert.Equal(
		t,
		rawSecret,
		svc.configureInput.Secret,
	)

	// HTTP response must never expose it.
	assert.NotContains(
		t,
		w.Body.String(),
		rawSecret,
	)

	assert.NotContains(
		t,
		w.Body.String(),
		"secret_arn",
	)

	assert.NotContains(
		t,
		w.Body.String(),
		"secret_version_id",
	)

	assert.Contains(
		t,
		w.Body.String(),
		`"secret_configured":true`,
	)
}

func TestProjectJWTConfigHandler_ValidationError(
	t *testing.T,
) {
	svc := &fakeProjectJWTHandlerService{
		configureErr: service.ErrJWTSecretTooShort,
	}

	router :=
		newProjectJWTHandlerTestRouter(svc)

	body := `{
		"enabled": true,
		"algorithm": "HS256",
		"secret": "weak"
	}`

	req := httptest.NewRequest(
		http.MethodPut,
		"/projects/project-a/security/jwt",
		strings.NewReader(body),
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(
		t,
		http.StatusBadRequest,
		w.Code,
	)
}

func TestProjectJWTConfigHandler_Delete(
	t *testing.T,
) {
	svc :=
		&fakeProjectJWTHandlerService{}

	router :=
		newProjectJWTHandlerTestRouter(svc)

	req := httptest.NewRequest(
		http.MethodDelete,
		"/projects/project-a/security/jwt",
		nil,
	)

	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(
		t,
		http.StatusNoContent,
		w.Code,
	)

	assert.Equal(
		t,
		1,
		svc.deleteCalls,
	)
}

func TestProjectJWTConfigHandler_DeleteNotFound(
	t *testing.T,
) {
	svc := &fakeProjectJWTHandlerService{
		deleteErr: storage.ErrProjectJWTConfigNotFound,
	}

	router :=
		newProjectJWTHandlerTestRouter(svc)

	req := httptest.NewRequest(
		http.MethodDelete,
		"/projects/project-a/security/jwt",
		nil,
	)

	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(
		t,
		http.StatusNotFound,
		w.Code,
	)
}

func TestProjectJWTConfigHandler_RequestTooLarge(
	t *testing.T,
) {
	svc :=
		&fakeProjectJWTHandlerService{}

	router :=
		newProjectJWTHandlerTestRouter(svc)

	hugeSecret :=
		strings.Repeat(
			"x",
			int(maxJWTConfigBodyBytes)+1024,
		)

	body := map[string]any{
		"enabled":   true,
		"algorithm": "HS256",
		"secret":    hugeSecret,
	}

	payload, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPut,
		"/projects/project-a/security/jwt",
		bytes.NewReader(payload),
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(
		t,
		http.StatusRequestEntityTooLarge,
		w.Code,
	)

	assert.Equal(
		t,
		0,
		svc.configureCalls,
	)
}

func TestProjectJWTConfigHandler_InternalError(
	t *testing.T,
) {
	svc := &fakeProjectJWTHandlerService{
		getErr: errors.New("database unavailable"),
	}

	router :=
		newProjectJWTHandlerTestRouter(svc)

	req := httptest.NewRequest(
		http.MethodGet,
		"/projects/project-a/security/jwt",
		nil,
	)

	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(
		t,
		http.StatusInternalServerError,
		w.Code,
	)
}
