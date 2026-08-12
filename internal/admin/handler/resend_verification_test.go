package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"elitegate/internal/admin/handler"
	adminmw "elitegate/internal/admin/middleware"
	authpkg "elitegate/internal/auth"
	"elitegate/internal/model"
	"elitegate/internal/ratelimit"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockResendRepo struct {
	userToReturn *model.AdminUser
	findErr      error

	calledReplaceToken bool
	capturedUserID     string
	capturedTokenHash  string
	capturedExpiresAt  time.Time

	calledInvalidateToken      bool
	capturedInvalidatedTokenID string
}

func (m *mockResendRepo) FindAdminUserByEmail(ctx context.Context, email string) (*model.AdminUser, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.userToReturn, nil
}
func (m *mockResendRepo) FindAdminUserByID(ctx context.Context, userID string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockResendRepo) FindAdminUserByUsername(ctx context.Context, username string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockResendRepo) FindAdminUserByGoogleID(ctx context.Context, googleID string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockResendRepo) CreateAdminUser(ctx context.Context, username, passwordHash string, isSuperAdmin bool) (*model.AdminUser, error) {
	return nil, nil
}
func (m *mockResendRepo) LinkGoogleAccount(ctx context.Context, userID, googleID, avatarURL string) error {
	return nil
}
func (m *mockResendRepo) SignupTx(ctx context.Context, username, email, passwordHash, companyName, slug, plan string, verificationTokenHash string, verificationExpiresAt time.Time) (*storage.SignupResult, error) {
	return nil, nil
}
func (m *mockResendRepo) GoogleSignupTx(ctx context.Context, email, googleID, displayName, avatarURL, companyName, slug string) (*storage.SignupResult, error) {
	return nil, nil
}
func (m *mockResendRepo) ReplacePasswordResetTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error) {
	return "", nil
}
func (m *mockResendRepo) ReplaceEmailVerificationTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error) {
	m.calledReplaceToken = true
	m.capturedUserID = adminUserID
	m.capturedTokenHash = tokenHash
	m.capturedExpiresAt = expiresAt
	return "resend_token_id_999", nil
}
func (m *mockResendRepo) InvalidatePasswordResetTokenByID(ctx context.Context, tokenID string) error {
	return nil
}
func (m *mockResendRepo) InvalidateEmailVerificationTokenByID(ctx context.Context, tokenID string) error {
	m.calledInvalidateToken = true
	m.capturedInvalidatedTokenID = tokenID
	return nil
}
func (m *mockResendRepo) FindValidEmailVerificationToken(ctx context.Context, tokenHash string) (*model.EmailVerificationToken, error) {
	return nil, storage.ErrInvalidEmailVerificationToken
}
func (m *mockResendRepo) VerifyEmailTx(ctx context.Context, tokenHash string) error {
	return nil
}
func (m *mockResendRepo) FindValidPasswordResetToken(ctx context.Context, tokenHash string) (*model.PasswordResetToken, error) {
	return nil, storage.ErrInvalidPasswordResetToken
}
func (m *mockResendRepo) ResetPasswordTx(ctx context.Context, resetTokenID, adminUserID, newPasswordHash string) error {
	return nil
}
func (m *mockResendRepo) IncrementAdminLoginFailure(ctx context.Context, username string) error {
	return nil
}
func (m *mockResendRepo) LockAdminUser(ctx context.Context, username string, until time.Time) error {
	return nil
}
func (m *mockResendRepo) UpdateAdminLoginSuccess(ctx context.Context, userID string) error {
	return nil
}
func (m *mockResendRepo) FindRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	return nil, sql.ErrNoRows
}
func (m *mockResendRepo) RotateRefreshToken(ctx context.Context, oldHash, newHash, userID string, exp time.Time, ip, ua string) error {
	return nil
}
func (m *mockResendRepo) CreateRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time, ipAddress, userAgent string) error {
	return nil
}
func (m *mockResendRepo) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	return nil
}
func (m *mockResendRepo) AdminUserCount(ctx context.Context) (int, error) {
	return 0, nil
}
func (m *mockResendRepo) IsSuperAdmin(ctx context.Context, userID string) (bool, error) {
	return false, nil
}

type mockResendMailer struct {
	sentRecipient       string
	sentVerificationURL string
	sendCount           int
	shouldFail          bool
}

func (m *mockResendMailer) SendPasswordReset(ctx context.Context, recipient, resetURL string) error {
	return nil
}

func (m *mockResendMailer) SendEmailVerification(ctx context.Context, recipient, verificationURL string) error {
	if m.shouldFail {
		return assert.AnError
	}
	m.sentRecipient = recipient
	m.sentVerificationURL = verificationURL
	m.sendCount++
	return nil
}

func TestResendVerificationHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	expectedMsg := "If an unverified account exists for that email, a verification link has been sent."

	t.Run("unknown email returns generic 200 and issues no token", func(t *testing.T) {
		repo := &mockResendRepo{findErr: sql.ErrNoRows}
		mailer := &mockResendMailer{}

		h := handler.NewAuthHandler(
			repo,
			nil,
			adminmw.NewLoginRateLimiter(5, 60),
			mailer,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/resend-verification", h.ResendVerification)

		body := map[string]string{"email": "unknown@example.com"}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/resend-verification", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.False(t, repo.calledReplaceToken)
		assert.Equal(t, 0, mailer.sendCount)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, expectedMsg, resp["message"])
	})

	t.Run("already verified user returns generic 200 and issues no token", func(t *testing.T) {
		repo := &mockResendRepo{
			userToReturn: &model.AdminUser{
				ID:            "user_verified",
				Email:         "verified@example.com",
				EmailVerified: true,
			},
		}
		mailer := &mockResendMailer{}

		h := handler.NewAuthHandler(
			repo,
			nil,
			adminmw.NewLoginRateLimiter(5, 60),
			mailer,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/resend-verification", h.ResendVerification)

		body := map[string]string{"email": "verified@example.com"}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/resend-verification", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.False(t, repo.calledReplaceToken, "already verified user must NOT get a new token")
		assert.Equal(t, 0, mailer.sendCount)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, expectedMsg, resp["message"])
	})

	t.Run("unverified user gets fresh token and email, returning generic 200", func(t *testing.T) {
		repo := &mockResendRepo{
			userToReturn: &model.AdminUser{
				ID:            "user_unverified",
				Email:         "unverified@example.com",
				EmailVerified: false,
			},
		}
		mailer := &mockResendMailer{}

		h := handler.NewAuthHandler(
			repo,
			nil,
			adminmw.NewLoginRateLimiter(5, 60),
			mailer,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/resend-verification", h.ResendVerification)

		body := map[string]string{"email": "UNVERIFIED@EXAMPLE.COM"}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/resend-verification", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, repo.calledReplaceToken)
		assert.Equal(t, "user_unverified", repo.capturedUserID)
		assert.NotEmpty(t, repo.capturedTokenHash)

		expectedMinExp := time.Now().UTC().Add(29 * time.Minute)
		expectedMaxExp := time.Now().UTC().Add(31 * time.Minute)
		assert.True(t, repo.capturedExpiresAt.After(expectedMinExp) && repo.capturedExpiresAt.Before(expectedMaxExp))

		assert.Equal(t, 1, mailer.sendCount)
		assert.Equal(t, "unverified@example.com", mailer.sentRecipient)
		assert.NotEmpty(t, mailer.sentVerificationURL)

		parsedURL, err := url.Parse(mailer.sentVerificationURL)
		require.NoError(t, err)
		rawToken := parsedURL.Query().Get("token")
		assert.NotEmpty(t, rawToken)

		assert.Equal(t, authpkg.HashToken(rawToken), repo.capturedTokenHash)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, expectedMsg, resp["message"])
		assert.Nil(t, resp["access_token"])

		cookies := w.Result().Cookies()
		for _, c := range cookies {
			assert.NotEqual(t, "admin_refresh_token", c.Name)
		}
	})

	t.Run("SMTP failure invalidates token and returns generic 200 response", func(t *testing.T) {
		repo := &mockResendRepo{
			userToReturn: &model.AdminUser{
				ID:            "user_unverified_fail",
				Email:         "fail@example.com",
				EmailVerified: false,
			},
		}
		mailer := &mockResendMailer{shouldFail: true}

		h := handler.NewAuthHandler(
			repo,
			nil,
			adminmw.NewLoginRateLimiter(5, 60),
			mailer,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/resend-verification", h.ResendVerification)

		body := map[string]string{"email": "fail@example.com"}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/resend-verification", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, repo.calledReplaceToken)
		assert.True(t, repo.calledInvalidateToken, "token must be invalidated when SMTP send fails")
		assert.Equal(t, "resend_token_id_999", repo.capturedInvalidatedTokenID)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, expectedMsg, resp["message"])
	})

	t.Run("rate limit enforcement: exceeding RPM returns HTTP 429 and blocks email send", func(t *testing.T) {
		repo := &mockResendRepo{
			userToReturn: &model.AdminUser{
				ID:            "user_ratelimit",
				Email:         "ratelimit@example.com",
				EmailVerified: false,
			},
		}
		mailer := &mockResendMailer{}

		h := handler.NewAuthHandler(
			repo,
			nil,
			adminmw.NewLoginRateLimiter(5, 60),
			mailer,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		rpm := 2
		limiter := ratelimit.NewMemoryLimiter(rpm)
		limiter.StartCleanup(context.Background(), time.Minute)

		r := gin.New()
		r.POST(
			"/admin/resend-verification",
			adminmw.IPRateLimit(limiter, rpm, "resend-verification", false),
			h.ResendVerification,
		)

		body := map[string]string{"email": "ratelimit@example.com"}
		jsonBody, _ := json.Marshal(body)

		// First 2 requests within limit
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest("POST", "/admin/resend-verification", bytes.NewReader(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "192.168.1.100:12345"
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		}
		assert.Equal(t, 2, mailer.sendCount)

		// 3rd request exceeds limit -> returns 429
		req3 := httptest.NewRequest("POST", "/admin/resend-verification", bytes.NewReader(jsonBody))
		req3.Header.Set("Content-Type", "application/json")
		req3.RemoteAddr = "192.168.1.100:12345"
		w3 := httptest.NewRecorder()
		r.ServeHTTP(w3, req3)

		assert.Equal(t, http.StatusTooManyRequests, w3.Code)
		assert.Equal(t, 2, mailer.sendCount, "mailer must NOT be called when rate limit is exceeded")
	})
}
