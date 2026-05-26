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
	"time"

	adminmw "elitegate/internal/admin/middleware"
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

func (h *AuthHandler) internal(c *gin.Context, err error) {
	h.logger.Error().Err(err).Msg("admin auth internal error")

	c.JSON(http.StatusInternalServerError, gin.H{
		"error": "internal server error",
	})
}

