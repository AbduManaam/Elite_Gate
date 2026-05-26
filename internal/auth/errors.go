package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserLocked         = errors.New("admin user locked")
	ErrTokenMissing       = errors.New("token missing")
	ErrTokenExpired       = errors.New("token expired")
	ErrTokenRevoked       = errors.New("token revoked")
	ErrInvalidToken       = errors.New("invalid token")
	ErrRateLimited        = errors.New("rate limited")
)
