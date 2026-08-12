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
	"elitegate/internal/admin/middleware"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type mockAuthRepo struct {
	user                   *model.AdminUser
	replaceTokenCalls      int
	invalidateByIDCalls    int
	invalidationContextErr error
}

func (m *mockAuthRepo) FindAdminUserByEmail(ctx context.Context, email string) (*model.AdminUser, error) {
	if m.user != nil && m.user.Email == email {
		return m.user, nil
	}
	return nil, sql.ErrNoRows
}

func (m *mockAuthRepo) FindAdminUserByID(ctx context.Context, userID string) (*model.AdminUser, error) {
	if m.user != nil && m.user.ID == userID {
		return m.user, nil
	}
	return nil, sql.ErrNoRows
}

func (m *mockAuthRepo) FindAdminUserByUsername(ctx context.Context, username string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}

func (m *mockAuthRepo) FindAdminUserByGoogleID(ctx context.Context, googleID string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}

func (m *mockAuthRepo) CreateAdminUser(ctx context.Context, username, passwordHash string, isSuperAdmin bool) (*model.AdminUser, error) {
	return nil, nil
}

func (m *mockAuthRepo) LinkGoogleAccount(ctx context.Context, userID, googleID, avatarURL string) error {
	return nil
}

func (m *mockAuthRepo) SignupTx(ctx context.Context, username, email, passwordHash, companyName, slug, plan string, verificationTokenHash string, verificationExpiresAt time.Time) (*storage.SignupResult, error) {
	return nil, nil
}

func (m *mockAuthRepo) GoogleSignupTx(ctx context.Context, email, googleID, displayName, avatarURL, companyName, slug string) (*storage.SignupResult, error) {
	return nil, nil
}

func (m *mockAuthRepo) ReplacePasswordResetTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error) {
	m.replaceTokenCalls++
	return "mock-token-id", nil
}

func (m *mockAuthRepo) InvalidatePasswordResetTokenByID(ctx context.Context, tokenID string) error {
	m.invalidateByIDCalls++
	m.invalidationContextErr = ctx.Err()
	return nil
}

func (m *mockAuthRepo) InvalidateEmailVerificationTokenByID(ctx context.Context, tokenID string) error {
	return nil
}

func (m *mockAuthRepo) FindValidEmailVerificationToken(ctx context.Context, tokenHash string) (*model.EmailVerificationToken, error) {
	return nil, storage.ErrInvalidEmailVerificationToken
}

func (m *mockAuthRepo) VerifyEmailTx(ctx context.Context, tokenHash string) error {
	return nil
}

func (m *mockAuthRepo) ReplaceEmailVerificationTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error) {
	return "mock-verif-token-id", nil
}

func (m *mockAuthRepo) FindValidPasswordResetToken(ctx context.Context, tokenHash string) (*model.PasswordResetToken, error) {
	return nil, storage.ErrInvalidPasswordResetToken
}

func (m *mockAuthRepo) ResetPasswordTx(ctx context.Context, resetTokenID, adminUserID, newPasswordHash string) error {
	return nil
}

func (m *mockAuthRepo) IncrementAdminLoginFailure(ctx context.Context, username string) error {
	return nil
}

func (m *mockAuthRepo) LockAdminUser(ctx context.Context, username string, until time.Time) error {
	return nil
}

func (m *mockAuthRepo) UpdateAdminLoginSuccess(ctx context.Context, userID string) error {
	return nil
}

func (m *mockAuthRepo) FindRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	return nil, sql.ErrNoRows
}

func (m *mockAuthRepo) RotateRefreshToken(ctx context.Context, oldHash, newHash, userID string, exp time.Time, ip, ua string) error {
	return nil
}

func (m *mockAuthRepo) CreateRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time, ipAddress, userAgent string) error {
	return nil
}

func (m *mockAuthRepo) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	return nil
}

func (m *mockAuthRepo) AdminUserCount(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *mockAuthRepo) IsSuperAdmin(ctx context.Context, userID string) (bool, error) {
	return false, nil
}

type mockMailer struct {
	sentRecipient       string
	sentResetURL        string
	sentVerificationURL string
	shouldFail          bool
}

func (m *mockMailer) SendPasswordReset(ctx context.Context, recipient, resetURL string) error {
	if m.shouldFail {
		return assert.AnError
	}
	m.sentRecipient = recipient
	m.sentResetURL = resetURL
	return nil
}

func (m *mockMailer) SendEmailVerification(ctx context.Context, recipient, verificationURL string) error {
	if m.shouldFail {
		return assert.AnError
	}
	m.sentRecipient = recipient
	m.sentVerificationURL = verificationURL
	return nil
}

func TestForgotPasswordHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns generic 200 for unknown email without creating reset token", func(t *testing.T) {
		repo := &mockAuthRepo{user: nil}
		mailer := &mockMailer{}
		limiter := middleware.NewLoginRateLimiter(5, 60)
		h := handler.NewAuthHandler(repo, nil, limiter, mailer, "http://localhost:5173/reset-password", "http://localhost:5173/verify-email", zerolog.Nop(), false)

		r := gin.New()
		r.POST("/forgot-password", h.ForgotPassword)

		body, _ := json.Marshal(map[string]string{"email": "unknown@example.com"})
		req := httptest.NewRequest("POST", "/forgot-password", bytes.NewReader(body))
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Zero(t, repo.replaceTokenCalls, "no token should be created for unknown email")
		assert.Empty(t, mailer.sentRecipient)
	})

	t.Run("returns generic 200 for synthetic @elitegate.local email without token creation", func(t *testing.T) {
		realHash, err := bcrypt.GenerateFromPassword([]byte("CurrentPassword1!"), bcrypt.MinCost)
		require.NoError(t, err)

		syntheticUser := &model.AdminUser{
			ID:           "u1",
			Username:     "legacy",
			Email:        "legacy@elitegate.local",
			PasswordHash: sql.NullString{String: string(realHash), Valid: true},
		}
		repo := &mockAuthRepo{user: syntheticUser}
		mailer := &mockMailer{}
		limiter := middleware.NewLoginRateLimiter(5, 60)
		h := handler.NewAuthHandler(repo, nil, limiter, mailer, "http://localhost:5173/reset-password", "http://localhost:5173/verify-email", zerolog.Nop(), false)

		r := gin.New()
		r.POST("/forgot-password", h.ForgotPassword)

		body, _ := json.Marshal(map[string]string{"email": "legacy@elitegate.local"})
		req := httptest.NewRequest("POST", "/forgot-password", bytes.NewReader(body))
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Zero(t, repo.replaceTokenCalls, "no token should be created for synthetic email")
		assert.Empty(t, mailer.sentRecipient)
	})

	t.Run("invalidates token using independent context even when request context is cancelled", func(t *testing.T) {
		realHash, err := bcrypt.GenerateFromPassword([]byte("CurrentPassword1!"), bcrypt.MinCost)
		require.NoError(t, err)

		validUser := &model.AdminUser{
			ID:           "u2",
			Username:     "validuser",
			Email:        "valid@example.com",
			PasswordHash: sql.NullString{String: string(realHash), Valid: true},
		}
		repo := &mockAuthRepo{user: validUser}
		mailer := &mockMailer{shouldFail: true}
		limiter := middleware.NewLoginRateLimiter(5, 60)
		h := handler.NewAuthHandler(repo, nil, limiter, mailer, "http://localhost:5173/reset-password", "http://localhost:5173/verify-email", zerolog.Nop(), false)

		r := gin.New()
		r.POST("/forgot-password", h.ForgotPassword)

		body, _ := json.Marshal(map[string]string{"email": "valid@example.com"})
		req := httptest.NewRequest("POST", "/forgot-password", bytes.NewReader(body))

		// Simulate cancelled request context
		ctx, cancel := context.WithCancel(req.Context())
		cancel()
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 1, repo.replaceTokenCalls)
		assert.Equal(t, 1, repo.invalidateByIDCalls)
		assert.NoError(t, repo.invalidationContextErr, "invalidation context must NOT be cancelled by request cancellation")
	})
}
