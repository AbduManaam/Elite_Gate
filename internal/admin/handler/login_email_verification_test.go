package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"golang.org/x/crypto/bcrypt"
)

type mockLoginAuthRepo struct {
	users map[string]*model.AdminUser

	calledUpdateSuccess      bool
	calledIncrementFailure   bool
	calledCreateRefreshToken bool
	calledLockUser           bool
	lastIncrementedUsername  string
	lastLockedUsername       string
}

func newMockLoginAuthRepo() *mockLoginAuthRepo {
	return &mockLoginAuthRepo{
		users: make(map[string]*model.AdminUser),
	}
}

func (m *mockLoginAuthRepo) FindAdminUserByUsername(ctx context.Context, username string) (*model.AdminUser, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return u, nil
}
func (m *mockLoginAuthRepo) FindAdminUserByEmail(ctx context.Context, email string) (*model.AdminUser, error) {
	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return nil, sql.ErrNoRows
}
func (m *mockLoginAuthRepo) FindAdminUserByID(ctx context.Context, userID string) (*model.AdminUser, error) {
	for _, u := range m.users {
		if u.ID == userID {
			return u, nil
		}
	}
	return nil, sql.ErrNoRows
}
func (m *mockLoginAuthRepo) FindAdminUserByGoogleID(ctx context.Context, googleID string) (*model.AdminUser, error) {
	return nil, sql.ErrNoRows
}
func (m *mockLoginAuthRepo) CreateAdminUser(ctx context.Context, username, passwordHash string, isSuperAdmin bool) (*model.AdminUser, error) {
	return nil, nil
}
func (m *mockLoginAuthRepo) LinkGoogleAccount(ctx context.Context, userID, googleID, avatarURL string) error {
	return nil
}
func (m *mockLoginAuthRepo) SignupTx(ctx context.Context, username, email, passwordHash, companyName, slug, plan string, verificationTokenHash string, verificationExpiresAt time.Time) (*storage.SignupResult, error) {
	return nil, nil
}
func (m *mockLoginAuthRepo) GoogleSignupTx(ctx context.Context, email, googleID, displayName, avatarURL, companyName, slug string) (*storage.SignupResult, error) {
	return nil, nil
}
func (m *mockLoginAuthRepo) ReplacePasswordResetTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error) {
	return "", nil
}
func (m *mockLoginAuthRepo) ReplaceEmailVerificationTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error) {
	return "", nil
}
func (m *mockLoginAuthRepo) InvalidatePasswordResetTokenByID(ctx context.Context, tokenID string) error {
	return nil
}
func (m *mockLoginAuthRepo) InvalidateEmailVerificationTokenByID(ctx context.Context, tokenID string) error {
	return nil
}
func (m *mockLoginAuthRepo) FindValidEmailVerificationToken(ctx context.Context, tokenHash string) (*model.EmailVerificationToken, error) {
	return nil, storage.ErrInvalidEmailVerificationToken
}
func (m *mockLoginAuthRepo) VerifyEmailTx(ctx context.Context, tokenHash string) error {
	return nil
}
func (m *mockLoginAuthRepo) FindValidPasswordResetToken(ctx context.Context, tokenHash string) (*model.PasswordResetToken, error) {
	return nil, storage.ErrInvalidPasswordResetToken
}
func (m *mockLoginAuthRepo) ResetPasswordTx(ctx context.Context, resetTokenID, adminUserID, newPasswordHash string) error {
	return nil
}
func (m *mockLoginAuthRepo) IncrementAdminLoginFailure(ctx context.Context, username string) error {
	m.calledIncrementFailure = true
	m.lastIncrementedUsername = username
	return nil
}
func (m *mockLoginAuthRepo) LockAdminUser(ctx context.Context, username string, until time.Time) error {
	m.calledLockUser = true
	m.lastLockedUsername = username
	return nil
}
func (m *mockLoginAuthRepo) UpdateAdminLoginSuccess(ctx context.Context, userID string) error {
	m.calledUpdateSuccess = true
	return nil
}
func (m *mockLoginAuthRepo) FindRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	return nil, sql.ErrNoRows
}
func (m *mockLoginAuthRepo) RotateRefreshToken(ctx context.Context, oldHash, newHash, userID string, exp time.Time, ip, ua string) error {
	return nil
}
func (m *mockLoginAuthRepo) CreateRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time, ipAddress, userAgent string) error {
	m.calledCreateRefreshToken = true
	return nil
}
func (m *mockLoginAuthRepo) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	return nil
}
func (m *mockLoginAuthRepo) AdminUserCount(ctx context.Context) (int, error) {
	return len(m.users), nil
}
func (m *mockLoginAuthRepo) IsSuperAdmin(ctx context.Context, userID string) (bool, error) {
	return false, nil
}

func TestLoginEmailVerification(t *testing.T) {
	gin.SetMode(gin.TestMode)

	password := "Password123!"
	hashedPwd, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	validPwdHash := sql.NullString{String: string(hashedPwd), Valid: true}

	t.Run("correct password on unverified account returns 403 EMAIL_NOT_VERIFIED and issues no tokens", func(t *testing.T) {
		repo := newMockLoginAuthRepo()
		repo.users["unverified_user"] = &model.AdminUser{
			ID:            "usr_unverified",
			Username:      "unverified_user",
			Email:         "unverified@example.com",
			PasswordHash:  validPwdHash,
			EmailVerified: false,
		}

		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/login", h.Login)

		body := map[string]string{
			"username": "unverified_user",
			"password": password,
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/login", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "EMAIL_NOT_VERIFIED", resp["code"])
		assert.Equal(t, "Please verify your email before signing in.", resp["error"])
		assert.Nil(t, resp["access_token"])

		assert.False(t, repo.calledUpdateSuccess, "UpdateAdminLoginSuccess must NOT be called for unverified user")
		assert.False(t, repo.calledCreateRefreshToken, "CreateRefreshToken must NOT be called for unverified user")
		assert.False(t, repo.calledIncrementFailure, "IncrementAdminLoginFailure must NOT be called on correct password")

		cookies := w.Result().Cookies()
		for _, c := range cookies {
			assert.NotEqual(t, "admin_refresh_token", c.Name, "refresh cookie must NOT be issued")
		}
	})

	t.Run("correct password on verified account logs in successfully with 200 and tokens", func(t *testing.T) {
		repo := newMockLoginAuthRepo()
		repo.users["verified_user"] = &model.AdminUser{
			ID:            "usr_verified",
			Username:      "verified_user",
			Email:         "verified@example.com",
			PasswordHash:  validPwdHash,
			EmailVerified: true,
		}

		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/login", h.Login)

		body := map[string]string{
			"username": "verified_user",
			"password": password,
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/login", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.NotEmpty(t, resp["access_token"])
		assert.True(t, repo.calledUpdateSuccess)
		assert.True(t, repo.calledCreateRefreshToken)

		foundCookie := false
		for _, c := range w.Result().Cookies() {
			if c.Name == "eg_refresh_token" {
				foundCookie = true
			}
		}
		assert.True(t, foundCookie, "refresh cookie must be set for verified user")
	})

	t.Run("valid email + correct password logs in successfully", func(t *testing.T) {
		repo := newMockLoginAuthRepo()
		repo.users["email_user"] = &model.AdminUser{
			ID:            "usr_email",
			Username:      "email_user",
			Email:         "user@example.com",
			PasswordHash:  validPwdHash,
			EmailVerified: true,
		}

		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/login", h.Login)

		body := map[string]string{
			"username": "user@example.com",
			"password": password,
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/login", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.NotEmpty(t, resp["access_token"])
		assert.True(t, repo.calledUpdateSuccess)
		assert.True(t, repo.calledCreateRefreshToken)
	})

	t.Run("mixed-case email + correct password logs in successfully (case-normalized)", func(t *testing.T) {
		repo := newMockLoginAuthRepo()
		repo.users["cased_user"] = &model.AdminUser{
			ID:            "usr_cased",
			Username:      "cased_user",
			Email:         "User@Example.com",
			PasswordHash:  validPwdHash,
			EmailVerified: true,
		}

		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/login", h.Login)

		body := map[string]string{
			"username": "USER@EXAMPLE.COM",
			"password": password,
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/login", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.NotEmpty(t, resp["access_token"])
		assert.True(t, repo.calledUpdateSuccess)
	})

	t.Run("wrong password on email login returns generic 401 and updates failure counter for canonical username", func(t *testing.T) {
		repo := newMockLoginAuthRepo()
		repo.users["email_user"] = &model.AdminUser{
			ID:            "usr_email",
			Username:      "email_user",
			Email:         "user@example.com",
			PasswordHash:  validPwdHash,
			EmailVerified: true,
		}

		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/login", h.Login)

		body := map[string]string{
			"username": "user@example.com",
			"password": "WrongPassword123!",
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/login", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "invalid username or password", resp["error"])
		assert.True(t, repo.calledIncrementFailure)
		assert.Equal(t, "email_user", repo.lastIncrementedUsername, "must increment failure for canonical username")
	})

	t.Run("unverified email account returning 403 EMAIL_NOT_VERIFIED on email login", func(t *testing.T) {
		repo := newMockLoginAuthRepo()
		repo.users["email_unverified"] = &model.AdminUser{
			ID:            "usr_email_unverified",
			Username:      "email_unverified",
			Email:         "unverified_email@example.com",
			PasswordHash:  validPwdHash,
			EmailVerified: false,
		}

		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/login", h.Login)

		body := map[string]string{
			"username": "unverified_email@example.com",
			"password": password,
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/login", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "EMAIL_NOT_VERIFIED", resp["code"])
		assert.Equal(t, "Please verify your email before signing in.", resp["error"])
		assert.Nil(t, resp["access_token"])
	})

	t.Run("unknown email returns generic 401", func(t *testing.T) {
		repo := newMockLoginAuthRepo()
		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/login", h.Login)

		body := map[string]string{
			"username": "unknown@example.com",
			"password": password,
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/login", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "invalid username or password", resp["error"])
		assert.Nil(t, resp["code"])
	})

	t.Run("wrong password on unverified account returns generic 401, NOT EMAIL_NOT_VERIFIED", func(t *testing.T) {
		repo := newMockLoginAuthRepo()
		repo.users["unverified_user"] = &model.AdminUser{
			ID:            "usr_unverified",
			Username:      "unverified_user",
			Email:         "unverified@example.com",
			PasswordHash:  validPwdHash,
			EmailVerified: false,
		}

		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/login", h.Login)

		body := map[string]string{
			"username": "unverified_user",
			"password": "WrongPassword123!",
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/login", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "invalid username or password", resp["error"])
		assert.Nil(t, resp["code"], "must NOT reveal EMAIL_NOT_VERIFIED when password is wrong")
		assert.True(t, repo.calledIncrementFailure, "wrong password must increment failure count")
	})

	t.Run("unknown user returns generic 401, NOT EMAIL_NOT_VERIFIED", func(t *testing.T) {
		repo := newMockLoginAuthRepo()
		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			adminmw.NewLoginRateLimiter(5, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/login", h.Login)

		body := map[string]string{
			"username": "nonexistent_user",
			"password": password,
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/login", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "invalid username or password", resp["error"])
		assert.Nil(t, resp["code"])
	})

	t.Run("account lockout occurs on 5th failed password attempt with email identifier", func(t *testing.T) {
		repo := newMockLoginAuthRepo()
		repo.users["lockout_user"] = &model.AdminUser{
			ID:                  "usr_lockout",
			Username:            "lockout_user",
			Email:               "lockout@example.com",
			PasswordHash:        validPwdHash,
			EmailVerified:       true,
			FailedLoginAttempts: 4, // 5th failure will trigger lock
		}

		tokens, err := authpkg.NewAdminTokenManager("supersecretjwtkey_32byteslongkey!", "test")
		require.NoError(t, err)

		h := handler.NewAuthHandler(
			repo,
			tokens,
			adminmw.NewLoginRateLimiter(100, 60),
			nil,
			"http://localhost:5173/reset-password",
			"http://localhost:5173/verify-email",
			zerolog.Nop(),
			false,
		)

		r := gin.New()
		r.POST("/admin/login", h.Login)

		body := map[string]string{
			"username": "lockout@example.com",
			"password": "WrongPassword123!",
		}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/admin/login", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.True(t, repo.calledLockUser, "LockAdminUser must be called on 5th failure")
		assert.Equal(t, "lockout_user", repo.lastLockedUsername, "LockAdminUser must lock canonical username")
	})
}
