package auth

import (
	"context"
	"errors"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
)

type GoogleOAuth struct {
	oauthCfg *oauth2.Config
}

func NewGoogleOAuth(clientID, clientSecret, redirectURL string) *GoogleOAuth {
	return &GoogleOAuth{
		oauthCfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
	}
}

// AuthCodeURL builds the URL to redirect the browser to, including the
// PKCE code_challenge and our signed CSRF state.
func (g *GoogleOAuth) AuthCodeURL(state, codeChallenge string) string {
	return g.oauthCfg.AuthCodeURL(
		state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)
}

// GoogleIdentity is the subset of ID token claims we trust and use.
type GoogleIdentity struct {
	Subject       string // Google's permanent, unique user ID
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

var ErrEmailNotVerified = errors.New("google account email is not verified")

// Exchange trades the authorization code (+ PKCE verifier) for Google
// tokens, verifies the ID token's signature/claims, and returns the
// user's verified identity.
func (g *GoogleOAuth) Exchange(ctx context.Context, code, codeVerifier string) (*GoogleIdentity, error) {
	token, err := g.oauthCfg.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		return nil, err
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("google token response missing id_token")
	}

	payload, err := idtoken.Validate(ctx, rawIDToken, g.oauthCfg.ClientID)
	if err != nil {
		return nil, err // signature invalid, expired, wrong audience, etc.
	}

	emailVerified, _ := payload.Claims["email_verified"].(bool)
	identity := &GoogleIdentity{
		Subject:       payload.Subject,
		Email:         asString(payload.Claims["email"]),
		EmailVerified: emailVerified,
		Name:          asString(payload.Claims["name"]),
		Picture:       asString(payload.Claims["picture"]),
	}

	if !identity.EmailVerified {
		return nil, ErrEmailNotVerified
	}
	return identity, nil
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}
