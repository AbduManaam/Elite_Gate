package password

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// MinLength is the minimum number of characters required for an admin password.
const MinLength = 8

// MaxLength mirrors bcrypt's hard truncation limit.
const MaxLength = 72

// WeakPasswords is a blocklist of commonly guessed passwords.
var WeakPasswords = []string{
	"admin123", "password123", "123456789012", "qwerty123456",
	"admin123456", "letmein123456", "welcome123456",
}

// Validate enforces all password strength rules.
// This is the single source of truth used by:
//   - POST /admin/register  (HTTP endpoint)
//   - POST /admin/v1/admins (HTTP endpoint)
func Validate(password string) error {

	// ── Length ────────────────────────────────────────────────────────
	if len(password) < MinLength {
		return fmt.Errorf("password must be at least %d characters", MinLength)
	}
	if len(password) > MaxLength {
		return fmt.Errorf("password must not exceed %d characters", MaxLength)
	}

	// ── Blocklist ─────────────────────────────────────────────────────
	for _, weak := range WeakPasswords {
		if strings.EqualFold(password, weak) {
			return fmt.Errorf("password %q is too common — choose a stronger one", password)
		}
	}

	// ── Complexity ────────────────────────────────────────────────────
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter (A-Z)")
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter (a-z)")
	}
	if !hasDigit {
		return errors.New("password must contain at least one digit (0-9)")
	}
	if !hasSpecial {
		return errors.New("password must contain at least one special character (!@#$%^&* etc.)")
	}

	return nil
}
