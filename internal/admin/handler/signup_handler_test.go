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
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSignupAuthRepo struct {
	calledSignupTx         bool
	capturedUsername       string
	capturedEmail          string
	capturedCompany        string
	capturedTokenHash      string
	capturedTokenExpiresAt time.Time

	calledInvalidateToken      bool
	capturedInvalidatedTokenID string
	invalidateTokenErr         error

	calledGoogleSignupTx bool
}

func (m *mockSignupAuthRepo) FindAdminUserByEmail(ctx context.Context, email string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockSignupAuthRepo) FindAdminUserByID(ctx context.Context, userID string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockSignupAuthRepo) FindAdminUserByUsername(ctx context.Context, username string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockSignupAuthRepo) FindAdminUserByGoogleID(ctx context.Context, googleID string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockSignupAuthRepo) CreateAdminUser(ctx context.Context, username, passwordHash string, isSuperAdmin bool) (*model.AdminUser, error) {
	return nil, nil
}
func (m *mockSignupAuthRepo) LinkGoogleAccount(ctx context.Context, userID, googleID, avatarURL string) error {
	return nil
}
func (m *mockSignupAuthRepo) SignupTx(ctx context.Context, username, email, passwordHash, companyName, slug, plan string, verificationTokenHash string, verificationExpiresAt time.Time) (*storage.SignupResult, error) {
	m.calledSignupTx = true
	m.capturedUsername = username
	m.capturedEmail = email
	m.capturedCompany = companyName
	m.capturedTokenHash = verificationTokenHash
	m.capturedTokenExpiresAt = verificationExpiresAt

	return &storage.SignupResult{
		User: model.AdminUser{
			ID:            "usr_123",
			Username:      username,
			Email:         email,
			EmailVerified: false,
		},
		Project: model.Project{
			ID:   "proj_456",
			Name: companyName,
			Slug: slug,
		},
		VerificationTokenID: "vtoken_789",
	}, nil
}
func (m *mockSignupAuthRepo) GoogleSignupTx(ctx context.Context, email, googleID, displayName, avatarURL, companyName, slug string) (*storage.SignupResult, error) {
	m.calledGoogleSignupTx = true
	return &storage.SignupResult{
		User: model.AdminUser{
			ID:            "usr_google",
			Username:      email,
			Email:         email,
			EmailVerified: true,
		},
		Project: model.Project{
			ID:   "proj_google",
			Name: companyName,
			Slug: slug,
		},
	}, nil
}
func (m *mockSignupAuthRepo) ReplacePasswordResetTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error) {
	return "", nil
}
func (m *mockSignupAuthRepo) InvalidatePasswordResetTokenByID(ctx context.Context, tokenID string) error {
	return nil
}
func (m *mockSignupAuthRepo) InvalidateEmailVerificationTokenByID(ctx context.Context, tokenID string) error {
	m.calledInvalidateToken = true
	m.capturedInvalidatedTokenID = tokenID
	return m.invalidateTokenErr
}
func (m *mockSignupAuthRepo) FindValidEmailVerificationToken(ctx context.Context, tokenHash string) (*model.EmailVerificationToken, error) {
	return nil, storage.ErrInvalidEmailVerificationToken
}
func (m *mockSignupAuthRepo) VerifyEmailTx(ctx context.Context, tokenHash string) error {
	return nil
}
func (m *mockSignupAuthRepo) ReplaceEmailVerificationTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error) {
	return "mock-verif-token-id", nil
}
func (m *mockSignupAuthRepo) FindValidPasswordResetToken(ctx context.Context, tokenHash string) (*model.PasswordResetToken, error) {
	return nil, sql.ErrNoRows
}
func (m *mockSignupAuthRepo) ResetPasswordTx(ctx context.Context, resetTokenID, adminUserID, newPasswordHash string) error {
	return nil
}
func (m *mockSignupAuthRepo) IncrementAdminLoginFailure(ctx context.Context, username string) error {
	return nil
}
func (m *mockSignupAuthRepo) LockAdminUser(ctx context.Context, username string, until time.Time) error {
	return nil
}
func (m *mockSignupAuthRepo) UpdateAdminLoginSuccess(ctx context.Context, userID string) error {
	return nil
}
func (m *mockSignupAuthRepo) FindRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	return nil, sql.ErrNoRows
}
func (m *mockSignupAuthRepo) RotateRefreshToken(ctx context.Context, oldHash, newHash, userID string, exp time.Time, ip, ua string) error {
	return nil
}
func (m *mockSignupAuthRepo) CreateRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time, ipAddress, userAgent string) error {
	return nil
}
func (m *mockSignupAuthRepo) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	return nil
}
func (m *mockSignupAuthRepo) AdminUserCount(ctx context.Context) (int, error) {
	return 0, nil
}
func (m *mockSignupAuthRepo) IsSuperAdmin(ctx context.Context, userID string) (bool, error) {
	return false, nil
}

type mockSignupMailer struct {
	sentRecipient       string
	sentVerificationURL string
	sendCount           int
	shouldFail          bool
}

func (m *mockSignupMailer) SendPasswordReset(ctx context.Context, recipient, resetURL string) error {
	return nil
}

func (m *mockSignupMailer) SendEmailVerification(ctx context.Context, recipient, verificationURL string) error {
	if m.shouldFail {
		return assert.AnError
	}
	m.sentRecipient = recipient
	m.sentVerificationURL = verificationURL
	m.sendCount++
	return nil
}

func TestPasswordSignup_EmailVerificationFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("creates verification token, persists token hash, sends email, and returns 201 without access token", func(t *testing.T) {
		repo := &mockSignupAuthRepo{}
		mailer := &mockSignupMailer{}
		limiter := adminmw.NewLoginRateLimiter(5, 60)
		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			limiter,
			mailer,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/signup", h.Signup)

		body := map[string]string{
			"username": "newuser",
			"email":    "newuser@example.com",
			"password": "Password123!",
			"company":  "Acme Corp",
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/signup", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Nil(t, resp["access_token"], "access_token must NOT be present in signup response for unverified user")
		assert.Equal(t, "Account created. Please check your email to verify your account.", resp["message"])
		assert.Equal(t, "proj_456", resp["project_id"])

		assert.True(t, repo.calledSignupTx)
		assert.Equal(t, "newuser@example.com", repo.capturedEmail)
		assert.NotEmpty(t, repo.capturedTokenHash)

		expectedMinExp := time.Now().UTC().Add(29 * time.Minute)
		expectedMaxExp := time.Now().UTC().Add(31 * time.Minute)
		assert.True(t, repo.capturedTokenExpiresAt.After(expectedMinExp) && repo.capturedTokenExpiresAt.Before(expectedMaxExp), "token expires_at should be ~30m from now")

		assert.Equal(t, 1, mailer.sendCount)
		assert.Equal(t, "newuser@example.com", mailer.sentRecipient)
		assert.NotEmpty(t, mailer.sentVerificationURL)

		parsedURL, err := url.Parse(mailer.sentVerificationURL)
		require.NoError(t, err)
		rawToken := parsedURL.Query().Get("token")
		assert.NotEmpty(t, rawToken, "verification URL must contain raw token in query parameter")

		computedHash := authpkg.HashToken(rawToken)
		assert.Equal(t, computedHash, repo.capturedTokenHash, "persisted token_hash must match HashToken(rawToken)")
		assert.NotEqual(t, rawToken, repo.capturedTokenHash, "raw token must NEVER be stored directly")

		cookies := w.Result().Cookies()
		for _, c := range cookies {
			assert.NotEqual(t, "admin_refresh_token", c.Name, "refresh token cookie must NOT be set on unverified signup")
		}
	})

	t.Run("when SendEmailVerification fails, invalidates verification token, returns 503, does not delete account, does not issue tokens", func(t *testing.T) {
		repo := &mockSignupAuthRepo{}
		mailer := &mockSignupMailer{shouldFail: true}
		limiter := adminmw.NewLoginRateLimiter(5, 60)
		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			limiter,
			mailer,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/signup", h.Signup)

		body := map[string]string{
			"username": "smtpfaileduser",
			"email":    "faileduser@example.com",
			"password": "Password123!",
			"company":  "Fail Corp",
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/signup", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "account created, but verification email could not be sent", resp["error"])
		assert.Nil(t, resp["access_token"], "access_token must NOT be present when email delivery fails")

		assert.True(t, repo.calledSignupTx, "SignupTx should have committed user and project")
		assert.True(t, repo.calledInvalidateToken, "verification token should be invalidated upon SMTP failure")
		assert.Equal(t, "vtoken_789", repo.capturedInvalidatedTokenID, "exact verification token ID should be passed to invalidation method")

		cookies := w.Result().Cookies()
		for _, c := range cookies {
			assert.NotEqual(t, "admin_refresh_token", c.Name, "refresh token cookie must NOT be set on delivery failure")
		}
	})

	t.Run("when SendEmailVerification AND token invalidation both fail, still returns 503 and does not issue tokens", func(t *testing.T) {
		repo := &mockSignupAuthRepo{invalidateTokenErr: assert.AnError}
		mailer := &mockSignupMailer{shouldFail: true}
		limiter := adminmw.NewLoginRateLimiter(5, 60)
		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			limiter,
			mailer,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/signup", h.Signup)

		body := map[string]string{
			"username": "doublefailuser",
			"email":    "doublefail@example.com",
			"password": "Password123!",
			"company":  "Double Fail Corp",
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/signup", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "account created, but verification email could not be sent", resp["error"])
		assert.Nil(t, resp["access_token"])
		assert.True(t, repo.calledInvalidateToken)
	})
}
