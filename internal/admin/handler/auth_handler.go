package handler

// Login
//   ↓
// Validate password
//   ↓
// Issue access token + refresh token
//   ↓
// Refresh expired access tokens
//   ↓
// Logout / revoke refresh token


import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	adminmw "elitegate/internal/admin/middleware"
	pwdpkg "elitegate/internal/admin/password"
	authpkg "elitegate/internal/auth"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
)

const (
	maxLoginFailures = 5
	lockoutDuration = 15 * time.Minute
	maxAuthBodyBytes = 1 << 20
)

type AuthHandler struct {
	repo    *storage.AdminAuthRepo
	tokens  *authpkg.AdminTokenManager
	limiter *adminmw.LoginRateLimiter
	logger  zerolog.Logger
}

func NewAuthHandler(
	repo *storage.AdminAuthRepo,
	tokens *authpkg.AdminTokenManager,
	limiter *adminmw.LoginRateLimiter,
	logger zerolog.Logger,
) *AuthHandler {
	return &AuthHandler{
		repo:    repo,
		tokens:  tokens,
		limiter: limiter,
		logger:  logger,
	}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// registerRequest is the body for POST /admin/register and POST /admin/v1/admins.
type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required"`
}

// registerResponse is returned on successful registration.
type registerResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// signupRequest is the body for POST /admin/signup (permanent public self-service).
type signupRequest struct {
	Username    string `json:"username"  binding:"required,min=3,max=64"`
	Password    string `json:"password"  binding:"required"`
	CompanyName string `json:"company"   binding:"required,min=1,max=128"`
	Plan        string `json:"plan"` // optional, defaults to "free"
}

// signupResponse is returned after a successful self-service signup.
type signupResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	ProjectID    string `json:"project_id"`
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

	user, err := h.repo.FindAdminUserByUsername(
		c.Request.Context(),
		req.Username,
	)

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

	if bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(req.Password),
	) != nil {

		_ = h.repo.IncrementAdminLoginFailure(
			c.Request.Context(),
			req.Username,
		)

		h.limiter.RecordFailure(ip)

		if user.FailedLoginAttempts+1 >= maxLoginFailures {
			until := time.Now().Add(lockoutDuration)

			_ = h.repo.LockAdminUser(
				c.Request.Context(),
				req.Username,
				until,
			)

			h.logger.Warn().
				Str("username", req.Username).
				Str("ip", ip).
				Time("until", until).
				Msg("admin locked")
		}

		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid username or password",
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

	h.issueTokens(c, user.ID, user.Username)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxAuthBodyBytes,
	)

	var req refreshRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request",
		})
		return
	}

	oldHash := authpkg.HashToken(req.RefreshToken)

	token, err := h.repo.FindRefreshToken(
		c.Request.Context(),
		oldHash,
	)

	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid refresh token",
		})
		return
	}

	if err != nil {
		h.internal(c, err)
		return
	}

	if token.RevokedAt.Valid {
		h.logger.Warn().
			Str("admin_user_id", token.AdminUserID).
			Msg("revoked refresh token reused")

		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid refresh token",
		})
		return
	}

	if time.Now().After(token.ExpiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "refresh token expired",
		})
		return
	}

	newRaw, err := authpkg.GenerateRefreshToken()
	if err != nil {
		h.internal(c, err)
		return
	}

	newHash := authpkg.HashToken(newRaw)
	newExp := time.Now().Add(authpkg.RefreshTokenTTL)

	ip := adminmw.ClientIP(c)
	ua := c.Request.UserAgent()

	if err := h.repo.RotateRefreshToken(
		c.Request.Context(),
		oldHash,
		newHash,
		token.AdminUserID,
		newExp,
		ip,
		ua,
	); err != nil {
		h.internal(c, err)
		return
	}

	user, err := h.repo.FindAdminUserByID(
	c.Request.Context(),
	token.AdminUserID,
	)
	if err != nil {
		h.internal(c, err)
		return
	}

	access, err := h.tokens.CreateAdminAccessToken(
		token.AdminUserID,
		user.Username,
	)

	if err != nil {
		h.internal(c, err)
		return
	}

	c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  access,
		RefreshToken: newRaw,
		ExpiresIn:    int(authpkg.AccessTokenTTL.Seconds()),
		TokenType:    "Bearer",
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxAuthBodyBytes,
	)

	var req refreshRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request",
		})
		return
	}

	_ = h.repo.RevokeRefreshToken(
		c.Request.Context(),
		authpkg.HashToken(req.RefreshToken),
	)

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

func (h *AuthHandler) issueTokens(
	c *gin.Context,
	userID,
	username string,
) {
	access, err := h.tokens.CreateAdminAccessToken(userID, username)
	if err != nil {
		h.internal(c, err)
		return
	}

	refresh, err := authpkg.GenerateRefreshToken()
	if err != nil {
		h.internal(c, err)
		return
	}

	exp := time.Now().Add(authpkg.RefreshTokenTTL)

	if err := h.repo.CreateRefreshToken(
		c.Request.Context(),
		userID,
		authpkg.HashToken(refresh),
		exp,
		adminmw.ClientIP(c),
		c.Request.UserAgent(),
	); err != nil {
		h.internal(c, err)
		return
	}

	h.logger.Info().
		Str("admin_user_id", userID).
		Msg("admin login success")

	c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int(authpkg.AccessTokenTTL.Seconds()),
		TokenType:    "Bearer",
	})
}

// Register handles admin account creation.
//
// It serves TWO routes with ONE handler:
//
//	POST /admin/register     → public, only works when 0 admins exist (bootstrap)
//	POST /admin/v1/admins    → protected, requires a valid admin JWT
//
// IMPORTANT (SaaS onboarding model):
//
//	POST /admin/v1/admins is a PLATFORM-OPERATOR SUPPORT TOOL — not the normal
//	tenant onboarding path. Use it only for edge cases such as: manually
//	provisioning an account on a tenant's behalf for support/escalation reasons.
//	Normal tenant self-registration goes through POST /admin/signup, which
//	requires NO super-admin involvement.
//
// How it knows which route called it:
//
//	AdminAuth middleware sets admin_user_id in context for the protected route.
//	If that key is present → authenticated call → skip bootstrap check.
//	If that key is absent  → public call → enforce bootstrap gate.
func (h *AuthHandler) Register(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAuthBodyBytes)

	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	// ── Bootstrap gate ────────────────────────────────────────────────
	// If the caller is NOT authenticated (no admin_user_id in context),
	// this is the public /admin/register endpoint.
	// Only allow it when the DB has zero admin users.
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

	// ── Password strength ─────────────────────────────────────────────
	if err := pwdpkg.Validate(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// ── Hash password ─────────────────────────────────────────────────
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.internal(c, err)
		return
	}

	// ── Persist ───────────────────────────────────────────────────────
	// Bootstrap path: isSuperAdmin=true (first-ever admin = platform operator).
	// Authenticated path (POST /admin/v1/admins): isSuperAdmin=false (support account).
	// Note: isAuthenticated was already determined above (bootstrap gate check).
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

// Signup handles POST /admin/signup.
//
// This is a PERMANENT, public, unauthenticated endpoint.
// It requires NO super-admin involvement at any point — this is by design.
// Any company can call this endpoint to self-onboard onto the platform.
//
// Uses SignupTx to atomically:
//  1. Create an admin_user account (is_super_admin=FALSE — tenant, not operator)
//  2. Create a project owned by that user
//  3. Insert the owner project_members row
//
// If project creation fails, the admin_user row is rolled back —
// no orphaned accounts are possible.
func (h *AuthHandler) Signup(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAuthBodyBytes)

	var req signupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn().Err(err).Msg("signup: invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	// ── Password strength ─────────────────────────────────────────────
	if err := pwdpkg.Validate(req.Password); err != nil {
		h.logger.Warn().Str("username", req.Username).Msg("signup: weak password rejected")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// ── Hash password ─────────────────────────────────────────────────
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

	// ── Single atomic transaction: admin_user + project + membership ──────────
	// SignupTx rolls back ALL inserts if any step fails.
	// No orphaned admin_user rows are possible.
	result, err := h.repo.SignupTx(
		c.Request.Context(),
		req.Username, string(hash), req.CompanyName, slug, plan,
	)
	if errors.Is(err, sql.ErrNoRows) {
		h.logger.Info().Str("username", req.Username).Msg("signup: username already taken")
		c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Str("username", req.Username).Msg("signup: atomic signup transaction failed")
		h.internal(c, err)
		return
	}

	h.logger.Info().
		Str("username", result.User.Username).
		Str("admin_user_id", result.User.ID).
		Str("project_id", result.Project.ID).
		Str("company", req.CompanyName).
		Msg("new tenant self-registered via /signup")

	// ── Issue tokens — company is live immediately ───────────────────────
	access, err := h.tokens.CreateAdminAccessToken(result.User.ID, result.User.Username)
	if err != nil {
		h.logger.Error().Err(err).Str("admin_user_id", result.User.ID).Msg("signup: access token generation failed")
		h.internal(c, err)
		return
	}
	refresh, err := authpkg.GenerateRefreshToken()
	if err != nil {
		h.logger.Error().Err(err).Str("admin_user_id", result.User.ID).Msg("signup: refresh token generation failed")
		h.internal(c, err)
		return
	}
	exp := time.Now().Add(authpkg.RefreshTokenTTL)
	if err := h.repo.CreateRefreshToken(
		c.Request.Context(),
		result.User.ID,
		authpkg.HashToken(refresh),
		exp,
		adminmw.ClientIP(c),
		c.Request.UserAgent(),
	); err != nil {
		h.logger.Error().Err(err).Str("admin_user_id", result.User.ID).Msg("signup: refresh token persistence failed")
		h.internal(c, err)
		return
	}

	c.JSON(http.StatusCreated, signupResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int(authpkg.AccessTokenTTL.Seconds()),
		TokenType:    "Bearer",
		ProjectID:    result.Project.ID,
	})
}

// toSlug converts a human company name to a URL-safe slug.
// "Company B Corp" → "company-b-corp"
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
	h.logger.Error().Err(err).Msg("admin auth internal error")

	c.JSON(http.StatusInternalServerError, gin.H{
		"error": "internal server error",
	})
}

