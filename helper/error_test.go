package helper_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"elitegate/helper"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func TestRespondInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("default error message when empty string passed", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/test-path", nil)

		var buf bytes.Buffer
		logger := zerolog.New(&buf)

		helper.RespondInternalError(c, logger, errors.New("db failure"), "")

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", w.Code)
		}
		expectedBody := `{"error":"internal error"}`
		if w.Body.String() != expectedBody {
			t.Errorf("expected body %s, got %s", expectedBody, w.Body.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte("db failure")) {
			t.Errorf("expected log buffer to contain 'db failure', got: %s", buf.String())
		}
	})

	t.Run("custom error message and nil error", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/custom-path", nil)

		var buf bytes.Buffer
		logger := zerolog.New(&buf)

		helper.RespondInternalError(c, logger, nil, "failed to load gateways")

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", w.Code)
		}
		expectedBody := `{"error":"failed to load gateways"}`
		if w.Body.String() != expectedBody {
			t.Errorf("expected body %s, got %s", expectedBody, w.Body.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte("failed to load gateways")) {
			t.Errorf("expected log buffer to contain 'failed to load gateways', got: %s", buf.String())
		}
	})
}
