package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"elitegate/internal/admin/handler"
	adminmw "elitegate/internal/admin/middleware"
	authpkg "elitegate/internal/auth"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockVerifyEmailRepo struct {
	calledVerifyEmailTx bool
	capturedTokenHash   string
	usedTokens          map[string]bool
	userVerified        bool
	verifyTxErrToReturn error

	calledFindToken    bool
	validTokenToReturn *model.EmailVerificationToken
	findErrToReturn    error
}

func (m *mockVerifyEmailRepo) FindAdminUserByEmail(ctx context.Context, email string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockVerifyEmailRepo) FindAdminUserByID(ctx context.Context, userID string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockVerifyEmailRepo) FindAdminUserByUsername(ctx context.Context, username string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockVerifyEmailRepo) FindAdminUserByGoogleID(ctx context.Context, googleID string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockVerifyEmailRepo) CreateAdminUser(ctx context.Context, username, passwordHash string, isSuperAdmin bool) (*model.AdminUser, error) {
	return nil, nil
}
func (m *mockVerifyEmailRepo) LinkGoogleAccount(ctx context.Context, userID, googleID, avatarURL string) error {
	return nil
}
func (m *mockVerifyEmailRepo) SignupTx(ctx context.Context, username, email, passwordHash, companyName, slug, plan string, verificationTokenHash string, verificationExpiresAt time.Time) (*storage.SignupResult, error) {
	return nil, nil
}
func (m *mockVerifyEmailRepo) GoogleSignupTx(ctx context.Context, email, googleID, displayName, avatarURL, companyName, slug string) (*storage.SignupResult, error) {
	return nil, nil
}
func (m *mockVerifyEmailRepo) ReplacePasswordResetTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error) {
	return "", nil
}
func (m *mockVerifyEmailRepo) ReplaceEmailVerificationTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error) {
	return "mock-verif-token-id", nil
}
func (m *mockVerifyEmailRepo) InvalidatePasswordResetTokenByID(ctx context.Context, tokenID string) error {
	return nil
}
func (m *mockVerifyEmailRepo) InvalidateEmailVerificationTokenByID(ctx context.Context, tokenID string) error {
	return nil
}
func (m *mockVerifyEmailRepo) FindValidEmailVerificationToken(ctx context.Context, tokenHash string) (*model.EmailVerificationToken, error) {
	m.calledFindToken = true
	m.capturedTokenHash = tokenHash
	if m.findErrToReturn != nil {
		return nil, m.findErrToReturn
	}
	return m.validTokenToReturn, nil
}
func (m *mockVerifyEmailRepo) VerifyEmailTx(ctx context.Context, tokenHash string) error {
	m.calledVerifyEmailTx = true
	m.capturedTokenHash = tokenHash
	if m.verifyTxErrToReturn != nil {
		return m.verifyTxErrToReturn
	}
	if m.usedTokens == nil {
		m.usedTokens = make(map[string]bool)
	}
	if m.usedTokens[tokenHash] {
		return storage.ErrInvalidEmailVerificationToken
	}
	m.usedTokens[tokenHash] = true
	m.userVerified = true
	return nil
}
func (m *mockVerifyEmailRepo) FindValidPasswordResetToken(ctx context.Context, tokenHash string) (*model.PasswordResetToken, error) {
	return nil, storage.ErrInvalidPasswordResetToken
}
func (m *mockVerifyEmailRepo) ResetPasswordTx(ctx context.Context, resetTokenID, adminUserID, newPasswordHash string) error {
	return nil
}
func (m *mockVerifyEmailRepo) IncrementAdminLoginFailure(ctx context.Context, username string) error {
	return nil
}
func (m *mockVerifyEmailRepo) LockAdminUser(ctx context.Context, username string, until time.Time) error {
	return nil
}
func (m *mockVerifyEmailRepo) UpdateAdminLoginSuccess(ctx context.Context, userID string) error {
	return nil
}
func (m *mockVerifyEmailRepo) FindRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	return nil, sql.ErrNoRows
}
func (m *mockVerifyEmailRepo) RotateRefreshToken(ctx context.Context, oldHash, newHash, userID string, exp time.Time, ip, ua string) error {
	return nil
}
func (m *mockVerifyEmailRepo) CreateRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time, ipAddress, userAgent string) error {
	return nil
}
func (m *mockVerifyEmailRepo) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	return nil
}
func (m *mockVerifyEmailRepo) AdminUserCount(ctx context.Context) (int, error) {
	return 0, nil
}
func (m *mockVerifyEmailRepo) IsSuperAdmin(ctx context.Context, userID string) (bool, error) {
	return false, nil
}

func TestVerifyEmailHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("valid token is hashed before lookup, sets email_verified=true, and returns 200", func(t *testing.T) {
		rawToken := "sample_raw_verification_token_12345"
		expectedHash := authpkg.HashToken(rawToken)

		repo := &mockVerifyEmailRepo{}

		h := handler.NewAuthHandler(
			repo,
			nil,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/verify-email", h.VerifyEmail)

		body := map[string]string{"token": rawToken}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/verify-email", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, repo.calledVerifyEmailTx)
		assert.True(t, repo.userVerified, "user must be marked email_verified=true")
		assert.Equal(t, expectedHash, repo.capturedTokenHash, "raw token must be hashed before repository invocation")
		assert.NotEqual(t, rawToken, repo.capturedTokenHash, "raw token must NEVER be sent to repository")

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Email verified successfully.", resp["message"])
		assert.Nil(t, resp["access_token"], "access token must NOT be issued upon email verification")

		cookies := w.Result().Cookies()
		for _, c := range cookies {
			assert.NotEqual(t, "admin_refresh_token", c.Name, "refresh cookie must NOT be issued")
		}
	})

	t.Run("concurrency and single-use: token cannot be reused twice", func(t *testing.T) {
		rawToken := "single_use_token_999"
		repo := &mockVerifyEmailRepo{}

		h := handler.NewAuthHandler(
			repo,
			nil,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/verify-email", h.VerifyEmail)

		// First Request: succeeds
		body := map[string]string{"token": rawToken}
		jsonBody, _ := json.Marshal(body)
		req1 := httptest.NewRequest("POST", "/admin/verify-email", bytes.NewReader(jsonBody))
		req1.Header.Set("Content-Type", "application/json")
		w1 := httptest.NewRecorder()

		r.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusOK, w1.Code)

		// Second Request (same token): fails
		req2 := httptest.NewRequest("POST", "/admin/verify-email", bytes.NewReader(jsonBody))
		req2.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()

		r.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusBadRequest, w2.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w2.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "invalid or expired verification token", resp["error"])
	})

	t.Run("missing token returns 400", func(t *testing.T) {
		repo := &mockVerifyEmailRepo{}
		h := handler.NewAuthHandler(
			repo,
			nil,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/verify-email", h.VerifyEmail)

		body := map[string]string{"token": ""}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/verify-email", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.False(t, repo.calledVerifyEmailTx)
	})

	t.Run("invalid or expired token returns safe error", func(t *testing.T) {
		repo := &mockVerifyEmailRepo{
			verifyTxErrToReturn: storage.ErrInvalidEmailVerificationToken,
		}
		h := handler.NewAuthHandler(
			repo,
			nil,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/verify-email", h.VerifyEmail)

		body := map[string]string{"token": "nonexistent_token"}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/verify-email", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, "invalid or expired verification token", resp["error"])
		assert.False(t, repo.userVerified)
	})

	t.Run("tx failure/rollback returns safe 400 error without exposing internal error", func(t *testing.T) {
		repo := &mockVerifyEmailRepo{
			verifyTxErrToReturn: assert.AnError,
		}
		h := handler.NewAuthHandler(
			repo,
			nil,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/verify-email", h.VerifyEmail)

		body := map[string]string{"token": "some_token"}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/verify-email", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, "invalid or expired verification token", resp["error"])
		assert.False(t, repo.userVerified)
	})
}
