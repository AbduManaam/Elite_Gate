package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"elitegate/helper"
	adminmw "elitegate/internal/admin/middleware"
	pwdpkg "elitegate/internal/admin/password"
	authpkg "elitegate/internal/auth"
	"elitegate/internal/mailer"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
)

const (
	maxLoginFailures                  = 5
	lockoutDuration                   = 15 * time.Minute
	maxAuthBodyBytes                  = 1 << 20
	genericForgotResponse             = "If an account exists for that email address, password reset instructions have been sent."
	genericResendVerificationResponse = "If an unverified account exists for that email, a verification link has been sent."
)

type AuthRepository interface {
	FindAdminUserByEmail(ctx context.Context, email string) (*model.AdminUser, error)
	FindAdminUserByID(ctx context.Context, userID string) (*model.AdminUser, error)
	FindAdminUserByUsername(ctx context.Context, username string) (*model.AdminUser, error)
	FindAdminUserByGoogleID(ctx context.Context, googleID string) (*model.AdminUser, error)
	CreateAdminUser(ctx context.Context, username, passwordHash string, isSuperAdmin bool) (*model.AdminUser, error)
	LinkGoogleAccount(ctx context.Context, userID, googleID, avatarURL string) error
	SignupTx(ctx context.Context, username, email, passwordHash, companyName, slug, plan string, verificationTokenHash string, verificationExpiresAt time.Time) (*storage.SignupResult, error)
	GoogleSignupTx(ctx context.Context, email, googleID, displayName, avatarURL, companyName, slug string) (*storage.SignupResult, error)
	ReplacePasswordResetTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error)
	ReplaceEmailVerificationTokenTx(ctx context.Context, adminUserID, tokenHash string, expiresAt time.Time) (string, error)
	InvalidatePasswordResetTokenByID(ctx context.Context, tokenID string) error
	InvalidateEmailVerificationTokenByID(ctx context.Context, tokenID string) error
	FindValidEmailVerificationToken(ctx context.Context, tokenHash string) (*model.EmailVerificationToken, error)
	VerifyEmailTx(ctx context.Context, tokenHash string) error
	FindValidPasswordResetToken(ctx context.Context, tokenHash string) (*model.PasswordResetToken, error)
	ResetPasswordTx(ctx context.Context, resetTokenID, adminUserID, newPasswordHash string) error
	IncrementAdminLoginFailure(ctx context.Context, username string) error
	LockAdminUser(ctx context.Context, username string, until time.Time) error
	UpdateAdminLoginSuccess(ctx context.Context, userID string) error
	FindRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error)
	RotateRefreshToken(ctx context.Context, oldHash, newHash, userID string, exp time.Time, ip, ua string) error
	CreateRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time, ipAddress, userAgent string) error
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	AdminUserCount(ctx context.Context) (int, error)
	IsSuperAdmin(ctx context.Context, userID string) (bool, error)
}

type AuthHandler struct {
	repo                 AuthRepository
	tokens               *authpkg.AdminTokenManager
	limiter              *adminmw.LoginRateLimiter
	mailer               mailer.Mailer
	passwordResetURL     string
	emailVerificationURL string
	logger               zerolog.Logger
	secureCookies        bool

	oauthState      *authpkg.OAuthStateManager
	googleOAuth     *authpkg.GoogleOAuth
	frontendBaseURL string
}

func NewAuthHandler(
	repo AuthRepository,
	tokens *authpkg.AdminTokenManager,
	limiter *adminmw.LoginRateLimiter,
	mailer mailer.Mailer,
	passwordResetURL string,
	emailVerificationURL string,
	logger zerolog.Logger,
	secureCookies bool,
) *AuthHandler {
	return &AuthHandler{
		repo:                 repo,
		tokens:               tokens,
		limiter:              limiter,
		mailer:               mailer,
		passwordResetURL:     passwordResetURL,
		emailVerificationURL: emailVerificationURL,
		logger:               logger,
		secureCookies:        secureCookies,
	}
}

func (h *AuthHandler) EnableGoogleOAuth(
	state *authpkg.OAuthStateManager,
	google *authpkg.GoogleOAuth,
	frontendURL string,
) {
	h.oauthState = state
	h.googleOAuth = google
	h.frontendBaseURL = frontendURL
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required"`
}

type registerResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type signupRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=64"`
	Email       string `json:"email"    binding:"required,email"`
	Password    string `json:"password" binding:"required"`
	CompanyName string `json:"company"  binding:"required,min=1,max=128"`
	Plan        string `json:"plan"` // optional, defaults to "free"
}

type signupResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	ProjectID   string `json:"project_id"`
}

type forgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type verifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

type resendVerificationRequest struct {
	Email string `json:"email" binding:"required,email,max=254"`
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func getPQConstraint(err error) string {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Constraint
	}
	return ""
}

func (h *AuthHandler) Login(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxAuthBodyBytes,
	)

	ip := adminmw.ClientIP(c)

	if h.limiter.TooManyFailures(ip) {
		h.logger.Warn().
			Str("ip", ip).
			Msg("admin login rate limited")

		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "too many login attempts",
		})
		return
	}

	var req loginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request",
		})
		return
	}

	identifier := strings.TrimSpace(req.Username)

	var (
		user *model.AdminUser
		err  error
	)

	if strings.Contains(identifier, "@") {
		user, err = h.repo.FindAdminUserByEmail(
			c.Request.Context(),
			strings.ToLower(identifier),
		)
		if errors.Is(err, sql.ErrNoRows) {
			user, err = h.repo.FindAdminUserByUsername(
				c.Request.Context(),
				identifier,
			)
		}
	} else {
		user, err = h.repo.FindAdminUserByUsername(
			c.Request.Context(),
			identifier,
		)
	}

	if errors.Is(err, sql.ErrNoRows) {
		h.limiter.RecordFailure(ip)

		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid username or password",
		})
		return
	}

	if err != nil {
		h.internal(c, err)
		return
	}

	if user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
		c.JSON(http.StatusLocked, gin.H{
			"error": "admin account locked",
		})
		return
	}

	if !user.PasswordHash.Valid {
		h.limiter.RecordFailure(ip)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid username or password",
		})
		return
	}

	if bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash.String),
		[]byte(req.Password),
	) != nil {

		_ = h.repo.IncrementAdminLoginFailure(
			c.Request.Context(),
			user.Username,
		)

		h.limiter.RecordFailure(ip)

		if user.FailedLoginAttempts+1 >= maxLoginFailures {
			until := time.Now().Add(lockoutDuration)

			_ = h.repo.LockAdminUser(
				c.Request.Context(),
				user.Username,
				until,
			)

			h.logger.Warn().
				Str("username", user.Username).
				Str("ip", ip).
				Time("until", until).
				Msg("admin locked")
		}

		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid username or password",
		})
		return
	}

	if !user.EmailVerified {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Please verify your email before signing in.",
			"code":  "EMAIL_NOT_VERIFIED",
		})
		return
	}

	h.limiter.Reset(ip)

	if err := h.repo.UpdateAdminLoginSuccess(
		c.Request.Context(),
		user.ID,
	); err != nil {
		h.internal(c, err)
		return
	}

	tokens, err := h.issueTokensForUser(c, user.ID, user.Username)
	if err != nil {
		h.internal(c, err)
		return
	}

	h.logger.Info().
		Str("admin_user_id", user.ID).
		Msg("admin login success")

	c.JSON(http.StatusOK, tokenResponse{
		AccessToken: tokens.AccessToken,
		ExpiresIn:   tokens.ExpiresIn,
		TokenType:   "Bearer",
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	rawToken, err := readRefreshCookie(c)
	if err != nil || rawToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing refresh token"})
		return
	}

	oldHash := authpkg.HashToken(rawToken)

	token, err := h.repo.FindRefreshToken(c.Request.Context(), oldHash)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}
	if err != nil {
		h.internal(c, err)
		return
	}

	if token.RevokedAt.Valid {
		h.logger.Warn().Str("admin_user_id", token.AdminUserID).Msg("revoked refresh token reused")
		clearRefreshCookie(c, h.secureCookies)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	if time.Now().After(token.ExpiresAt) {
		clearRefreshCookie(c, h.secureCookies)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token expired"})
		return
	}

	newRaw, err := authpkg.GenerateRefreshToken()
	if err != nil {
		h.internal(c, err)
		return
	}

	newHash := authpkg.HashToken(newRaw)
	newExp := time.Now().Add(authpkg.RefreshTokenTTL)

	if err := h.repo.RotateRefreshToken(
		c.Request.Context(), oldHash, newHash, token.AdminUserID, newExp,
		adminmw.ClientIP(c), c.Request.UserAgent(),
	); err != nil {
		h.internal(c, err)
		return
	}

	user, err := h.repo.FindAdminUserByID(c.Request.Context(), token.AdminUserID)
	if err != nil {
		h.internal(c, err)
		return
	}

	access, err := h.tokens.CreateAdminAccessToken(token.AdminUserID, user.Username)
	if err != nil {
		h.internal(c, err)
		return
	}

	setRefreshCookie(c, newRaw, h.secureCookies)

	c.JSON(http.StatusOK, tokenResponse{
		AccessToken: access,
		ExpiresIn:   int(authpkg.AccessTokenTTL.Seconds()),
		TokenType:   "Bearer",
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	rawToken, err := readRefreshCookie(c)
	if err == nil && rawToken != "" {
		_ = h.repo.RevokeRefreshToken(c.Request.Context(), authpkg.HashToken(rawToken))
	}

	clearRefreshCookie(c, h.secureCookies)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *AuthHandler) Register(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAuthBodyBytes)

	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	_, isAuthenticated := c.Get(adminmw.AdminUserIDKey)
	if !isAuthenticated {
		count, err := h.repo.AdminUserCount(c.Request.Context())
		if err != nil {
			h.internal(c, err)
			return
		}
		if count > 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "registration is locked after bootstrap admin is created",
			})
			return
		}
	}

	if err := pwdpkg.Validate(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.internal(c, err)
		return
	}

	isSuperAdmin := !isAuthenticated
	user, err := h.repo.CreateAdminUser(c.Request.Context(), req.Username, string(hash), isSuperAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
		return
	}
	if err != nil {
		h.internal(c, err)
		return
	}

	h.logger.Info().
		Str("username", user.Username).
		Str("admin_user_id", user.ID).
		Msg("new admin user registered")

	c.JSON(http.StatusCreated, registerResponse{
		ID:       user.ID,
		Username: user.Username,
	})
}

func (h *AuthHandler) Signup(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAuthBodyBytes)

	var req signupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn().Err(err).Msg("signup: invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))

	if err := pwdpkg.Validate(req.Password); err != nil {
		h.logger.Warn().Str("username", req.Username).Msg("signup: weak password rejected")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error().Err(err).Str("username", req.Username).Msg("signup: password hashing failed")
		h.internal(c, err)
		return
	}

	slug := toSlug(req.CompanyName)
	plan := req.Plan
	if plan == "" {
		plan = "free"
	}

	rawToken, err := authpkg.GenerateEmailVerificationToken()
	if err != nil {
		h.logger.Error().Err(err).Str("username", req.Username).Msg("signup: failed to generate email verification token")
		h.internal(c, err)
		return
	}

	verificationTokenHash := authpkg.HashToken(rawToken)
	expiresAt := time.Now().UTC().Add(authpkg.EmailVerificationTokenTTL)

	result, err := h.repo.SignupTx(
		c.Request.Context(),
		req.Username, email, string(hash), req.CompanyName, slug, plan,
		verificationTokenHash, expiresAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			constraint := getPQConstraint(err)
			switch constraint {
			case "admin_users_email_unique":
				c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
				return
			case "idx_admin_users_username", "admin_users_username_key":
				c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
				return
			default:
				c.JSON(http.StatusConflict, gin.H{"error": "account details already registered"})
				return
			}
		}
		h.logger.Error().Err(err).Str("username", req.Username).Msg("signup: atomic signup transaction failed")
		h.internal(c, err)
		return
	}

	h.logger.Info().
		Str("username", result.User.Username).
		Str("admin_user_id", result.User.ID).
		Str("project_id", result.Project.ID).
		Str("company", req.CompanyName).
		Msg("new tenant self-registered via /signup (email verification pending)")

	verificationURL, err := url.Parse(h.emailVerificationURL)
	if err != nil {
		h.logger.Error().Err(err).Msg("signup: failed to parse email verification URL")
		h.internal(c, errors.New("failed to construct verification URL"))
		return
	}
	q := verificationURL.Query()
	q.Set("token", rawToken)
	verificationURL.RawQuery = q.Encode()

	if h.mailer != nil {
		if err := h.mailer.SendEmailVerification(
			c.Request.Context(),
			result.User.Email,
			verificationURL.String(),
		); err != nil {
			h.logger.Error().Err(err).Str("admin_user_id", result.User.ID).Msg("signup: failed to send email verification")
			h.invalidateFailedVerificationToken(result.VerificationTokenID, "smtp delivery failed")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "account created, but verification email could not be sent",
			})
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":    "Account created. Please check your email to verify your account.",
		"project_id": result.Project.ID,
	})
}

func (h *AuthHandler) invalidateFailedVerificationToken(tokenID, reason string) {
	if tokenID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := h.repo.InvalidateEmailVerificationTokenByID(ctx, tokenID); err != nil {
		h.logger.Error().Err(err).Str("token_id", tokenID).Str("reason", reason).Msg("signup: failed to invalidate email verification token after delivery error")
	}
}

func (h *AuthHandler) invalidateFailedToken(tokenID, reason string) {
	if tokenID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := h.repo.InvalidatePasswordResetTokenByID(ctx, tokenID); err != nil {
		h.logger.Error().Err(err).Str("token_id", tokenID).Str("reason", reason).Msg("forgot password: failed to invalidate token after delivery error")
	}
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAuthBodyBytes)

	var req forgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email format"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))

	user, err := h.repo.FindAdminUserByEmail(c.Request.Context(), email)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusOK, gin.H{"message": genericForgotResponse})
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("forgot password lookup failed")
		c.JSON(http.StatusOK, gin.H{"message": genericForgotResponse})
		return
	}

	if !user.PasswordHash.Valid ||
		strings.TrimSpace(user.PasswordHash.String) == "" ||
		strings.HasSuffix(strings.ToLower(user.Email), "@elitegate.local") {
		c.JSON(http.StatusOK, gin.H{"message": genericForgotResponse})
		return
	}

	if h.mailer == nil {
		h.logger.Error().Str("user_id", user.ID).Msg("forgot password: mailer is not configured")
		c.JSON(http.StatusOK, gin.H{"message": genericForgotResponse})
		return
	}

	rawToken, err := authpkg.GeneratePasswordResetToken()
	if err != nil {
		h.logger.Error().Err(err).Msg("forgot password: generate token failed")
		c.JSON(http.StatusOK, gin.H{"message": genericForgotResponse})
		return
	}

	tokenHash := authpkg.HashToken(rawToken)
	expiresAt := time.Now().UTC().Add(15 * time.Minute)

	tokenID, err := h.repo.ReplacePasswordResetTokenTx(c.Request.Context(), user.ID, tokenHash, expiresAt)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", user.ID).Msg("forgot password: transactional token replace failed")
		c.JSON(http.StatusOK, gin.H{"message": genericForgotResponse})
		return
	}

	parsedURL, err := url.ParseRequestURI(h.passwordResetURL)
	if err != nil {
		h.logger.Error().Err(err).Msg("forgot password: parse reset URL failed")
		h.invalidateFailedToken(tokenID, "url parse failed")
		c.JSON(http.StatusOK, gin.H{"message": genericForgotResponse})
		return
	}

	q := parsedURL.Query()
	q.Set("token", rawToken)
	parsedURL.RawQuery = q.Encode()

	mailCtx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	if err := h.mailer.SendPasswordReset(mailCtx, user.Email, parsedURL.String()); err != nil {
		h.logger.Error().Err(err).Str("user_id", user.ID).Msg("forgot password: send email failed")
		h.invalidateFailedToken(tokenID, "smtp delivery failed")
	}

	c.JSON(http.StatusOK, gin.H{"message": genericForgotResponse})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAuthBodyBytes)

	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	rawToken := strings.TrimSpace(req.Token)
	if rawToken == "" || len(rawToken) > 1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired password reset link"})
		return
	}

	if err := pwdpkg.Validate(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokenHash := authpkg.HashToken(rawToken)
	resetToken, err := h.repo.FindValidPasswordResetToken(c.Request.Context(), tokenHash)
	if errors.Is(err, storage.ErrInvalidPasswordResetToken) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired password reset link"})
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("reset password find token internal failure")
		h.internal(c, err)
		return
	}

	user, err := h.repo.FindAdminUserByID(c.Request.Context(), resetToken.AdminUserID)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", resetToken.AdminUserID).Msg("reset password: load user failed")
		h.internal(c, err)
		return
	}

	if user.PasswordHash.Valid && strings.TrimSpace(user.PasswordHash.String) != "" {
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), []byte(req.NewPassword)) == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "new password must be different from current password"})
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		h.internal(c, err)
		return
	}

	if err := h.repo.ResetPasswordTx(c.Request.Context(), resetToken.ID, resetToken.AdminUserID, string(hash)); err != nil {
		if errors.Is(err, storage.ErrInvalidPasswordResetToken) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired password reset link"})
			return
		}
		h.logger.Error().Err(err).Msg("reset password transaction failed")
		h.internal(c, err)
		return
	}

	clearRefreshCookie(c, h.secureCookies)

	c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully. Sign in using your new password."})
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAuthBodyBytes)

	var req verifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	rawToken := strings.TrimSpace(req.Token)
	if rawToken == "" || len(rawToken) > 1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired verification token"})
		return
	}

	tokenHash := authpkg.HashToken(rawToken)
	err := h.repo.VerifyEmailTx(c.Request.Context(), tokenHash)
	if errors.Is(err, storage.ErrInvalidEmailVerificationToken) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired verification token"})
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("verify email: transaction internal failure")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired verification token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully."})
}

func (h *AuthHandler) ResendVerification(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAuthBodyBytes)

	var req resendVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email address"})
		return
	}

	user, err := h.repo.FindAdminUserByEmail(c.Request.Context(), email)
	if errors.Is(err, sql.ErrNoRows) || user == nil {
		c.JSON(http.StatusOK, gin.H{"message": genericResendVerificationResponse})
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("resend verification: user lookup failed")
		c.JSON(http.StatusOK, gin.H{"message": genericResendVerificationResponse})
		return
	}

	if user.EmailVerified {
		c.JSON(http.StatusOK, gin.H{"message": genericResendVerificationResponse})
		return
	}

	rawToken, err := authpkg.GenerateEmailVerificationToken()
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", user.ID).Msg("resend verification: token generation failed")
		c.JSON(http.StatusOK, gin.H{"message": genericResendVerificationResponse})
		return
	}

	tokenHash := authpkg.HashToken(rawToken)
	expiresAt := time.Now().UTC().Add(authpkg.EmailVerificationTokenTTL)

	tokenID, err := h.repo.ReplaceEmailVerificationTokenTx(c.Request.Context(), user.ID, tokenHash, expiresAt)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", user.ID).Msg("resend verification: token replacement tx failed")
		c.JSON(http.StatusOK, gin.H{"message": genericResendVerificationResponse})
		return
	}

	verificationURL, err := url.Parse(h.emailVerificationURL)
	if err != nil {
		h.logger.Error().Err(err).Msg("resend verification: failed to parse verification URL")
		h.invalidateFailedVerificationToken(tokenID, "url parse failed")
		c.JSON(http.StatusOK, gin.H{"message": genericResendVerificationResponse})
		return
	}
	q := verificationURL.Query()
	q.Set("token", rawToken)
	verificationURL.RawQuery = q.Encode()

	if h.mailer != nil {
		mailCtx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		if err := h.mailer.SendEmailVerification(mailCtx, user.Email, verificationURL.String()); err != nil {
			h.logger.Error().Err(err).Str("user_id", user.ID).Msg("resend verification: send email failed")
			h.invalidateFailedVerificationToken(tokenID, "smtp delivery failed")
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": genericResendVerificationResponse})
}

func toSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteRune('-')
			prevHyphen = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		result = "project"
	}
	return result
}

func (h *AuthHandler) internal(c *gin.Context, err error) {
	helper.RespondInternalError(c, h.logger, err, "internal server error")
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID, _ := c.Get(adminmw.AdminUserIDKey)
	username, _ := c.Get(adminmw.AdminUsernameKey)

	userIDStr, _ := userID.(string)
	usernameStr, _ := username.(string)

	isSuperAdmin, err := h.repo.IsSuperAdmin(c.Request.Context(), userIDStr)
	if err != nil {
		helper.RespondInternalError(c, h.logger.With().Str("user_id", userIDStr).Logger(), err, "failed to load user")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":        userIDStr,
		"username":       usernameStr,
		"is_super_admin": isSuperAdmin,
	})
}
