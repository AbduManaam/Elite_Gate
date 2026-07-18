package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	authpkg "elitegate/internal/auth"
	"elitegate/internal/model"

	"github.com/gin-gonic/gin"
)

const oauthStateCookie = "eg_oauth_flow"

// GoogleLogin handles GET /admin/google/login — step 1 of the flow.
// It generates PKCE + CSRF state, stashes them in a short-lived cookie
// (defense in depth alongside the signed state token itself), and
// redirects the browser to Google's consent screen.
func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	verifier, challenge, err := authpkg.GeneratePKCEPair()
	if err != nil {
		h.internal(c, err)
		return
	}

	state, err := h.oauthState.CreateState(verifier)
	if err != nil {
		h.internal(c, err)
		return
	}

	// Belt-and-suspenders: the state JWT is self-verifying, but we also
	// set it as a short-lived HttpOnly cookie so a callback request that
	// arrives without ANY matching cookie (e.g. a replayed/forged link
	// opened in a different browser) can be rejected outright.
	c.SetCookie(oauthStateCookie, state, 600, "/admin/google", "", true, true)

	c.Redirect(http.StatusFound, h.googleOAuth.AuthCodeURL(state, challenge))
}

// GoogleCallback handles GET /admin/google/callback — steps 6-12 of the flow.
func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	frontendURL := h.frontendBaseURL

	code := c.Query("code")
	state := c.Query("state")
	if code == "" || state == "" {
		h.redirectWithError(c, frontendURL, "missing code or state")
		return
	}

	cookieState, err := c.Cookie(oauthStateCookie)
	c.SetCookie(oauthStateCookie, "", -1, "/admin/google", "", true, true) // always clear it
	if err != nil || cookieState != state {
		h.logger.Warn().Msg("oauth callback: state cookie mismatch (possible CSRF)")
		h.redirectWithError(c, frontendURL, "invalid oauth state")
		return
	}

	codeVerifier, err := h.oauthState.ValidateState(state)
	if err != nil {
		h.logger.Warn().Err(err).Msg("oauth callback: state token invalid or expired")
		h.redirectWithError(c, frontendURL, "oauth session expired, please try again")
		return
	}

	identity, err := h.googleOAuth.Exchange(c.Request.Context(), code, codeVerifier)
	if err != nil {
		if errors.Is(err, authpkg.ErrEmailNotVerified) {
			h.redirectWithError(c, frontendURL, "your Google email is not verified")
			return
		}
		h.logger.Error().Err(err).Msg("oauth callback: google token exchange failed")
		h.redirectWithError(c, frontendURL, "google authentication failed")
		return
	}

	user, err := h.resolveGoogleUser(c, identity)
	if err != nil {
		h.logger.Error().Err(err).Msg("oauth callback: failed to resolve/create user")
		h.redirectWithError(c, frontendURL, "could not complete sign-in")
		return
	}

	if err := h.repo.UpdateAdminLoginSuccess(c.Request.Context(), user.ID); err != nil {
		h.internal(c, err)
		return
	}

	tokens, err := h.issueTokensForUser(c, user.ID, user.Username)
	if err != nil {
		h.internal(c, err)
		return
	}

	h.logger.Info().Str("admin_user_id", user.ID).Msg("admin google login success")

	redirectURL := frontendURL + "/oauth/callback#" +
		url.Values{
			"access_token": {tokens.AccessToken},
			"expires_in":   {strconv.Itoa(tokens.ExpiresIn)},
			"token_type":   {"Bearer"},
		}.Encode()

	c.Redirect(http.StatusFound, redirectURL)
}

// resolveGoogleUser implements the Case A / B / C decision tree from Part 5.
func (h *AuthHandler) resolveGoogleUser(c *gin.Context, id *authpkg.GoogleIdentity) (*model.AdminUser, error) {
	ctx := c.Request.Context()

	// Case A: returning Google user.
	if u, err := h.repo.FindAdminUserByGoogleID(ctx, id.Subject); err == nil {
		return u, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Case B: existing account with matching (verified) email — link it.
	if u, err := h.repo.FindAdminUserByEmail(ctx, id.Email); err == nil {
		if err := h.repo.LinkGoogleAccount(ctx, u.ID, id.Subject, id.Picture); err != nil {
			return nil, err
		}
		u.GoogleID = sql.NullString{String: id.Subject, Valid: true}
		return u, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Case C: brand new user — self-service tenant creation, same shape as /admin/signup.
	companyName := id.Name
	if companyName == "" {
		companyName = id.Email
	}
	slug := toSlug(companyName) // reuses the existing helper from auth_handler.go
	result, err := h.repo.GoogleSignupTx(ctx, id.Email, id.Subject, id.Name, id.Picture, companyName, slug)
	if err != nil {
		return nil, err
	}
	return &result.User, nil
}

func (h *AuthHandler) redirectWithError(c *gin.Context, frontendURL, msg string) {
	c.Redirect(http.StatusFound, frontendURL+"/login?oauth_error="+url.QueryEscape(msg))
}
