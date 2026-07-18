package handler

import (
	"net/http"
	"time"

	adminmw "elitegate/internal/admin/middleware"
	authpkg "elitegate/internal/auth"

	"github.com/gin-gonic/gin"
)

const refreshCookieName = "eg_refresh_token"

// setRefreshCookie writes the refresh token as an HttpOnly cookie. secure
// controls the Secure attribute — false only in local (non-TLS) dev, true
// everywhere else. SameSite=Lax (not Strict) so the cookie still attaches
// on the top-level GET redirect Google OAuth performs back to our own
// /oauth/callback page.
func setRefreshCookie(c *gin.Context, value string, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, value, int(authpkg.RefreshTokenTTL.Seconds()), "/", "", secure, true)
}

func clearRefreshCookie(c *gin.Context, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, "", -1, "/", "", secure, true)
}

func readRefreshCookie(c *gin.Context) (string, error) {
	return c.Cookie(refreshCookieName)
}

// issuedTokens is what every login-shaped flow (password login, signup,
// Google OAuth) produces. Callers decide how to hand accessToken to the
// client (JSON body vs. redirect fragment) — the refresh token never
// leaves this function except via the cookie.
type issuedTokens struct {
	AccessToken string
	ExpiresIn   int
}

// issueTokensForUser is the single place that creates a token pair,
// persists the refresh token, and sets the cookie. Login, Signup, and
// GoogleCallback all call this instead of each hand-rolling the same
// four steps.
func (h *AuthHandler) issueTokensForUser(c *gin.Context, userID, username string) (issuedTokens, error) {
	access, err := h.tokens.CreateAdminAccessToken(userID, username)
	if err != nil {
		return issuedTokens{}, err
	}

	refresh, err := authpkg.GenerateRefreshToken()
	if err != nil {
		return issuedTokens{}, err
	}

	exp := time.Now().Add(authpkg.RefreshTokenTTL)
	if err := h.repo.CreateRefreshToken(
		c.Request.Context(), userID, authpkg.HashToken(refresh), exp,
		adminmw.ClientIP(c), c.Request.UserAgent(),
	); err != nil {
		return issuedTokens{}, err
	}

	setRefreshCookie(c, refresh, h.secureCookies)

	return issuedTokens{
		AccessToken: access,
		ExpiresIn:   int(authpkg.AccessTokenTTL.Seconds()),
	}, nil
}
