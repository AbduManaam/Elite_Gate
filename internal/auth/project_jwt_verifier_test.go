package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const projectJWTTestSecret = "12345678901234567890123456789012"

func signProjectJWTForTest(
	t *testing.T,
	method jwt.SigningMethod,
	claims jwt.MapClaims,
	secret string,
) string {
	t.Helper()

	token := jwt.NewWithClaims(
		method,
		claims,
	)

	value, err := token.SignedString(
		[]byte(secret),
	)

	require.NoError(t, err)

	return value
}

func testProjectJWTVerifier(
	t *testing.T,
) *ProjectJWTVerifier {
	t.Helper()

	issuer := "https://auth.yumzy.test"

	verifier, err := NewProjectJWTVerifier(
		projectJWTTestSecret,
		ProjectJWTVerifierConfig{
			Algorithm: "HS256",

			Issuer: &issuer,

			Audiences: []string{
				"yumzy-api",
			},

			SubjectClaim: "sub",
			RoleClaim:    "role",
			ScopesClaim:  "scope",

			ClockSkew: 30 * time.Second,
		},
	)

	require.NoError(t, err)

	return verifier
}

func TestProjectJWTVerifierValidToken(
	t *testing.T,
) {
	verifier := testProjectJWTVerifier(t)

	token := signProjectJWTForTest(
		t,
		jwt.SigningMethodHS256,
		jwt.MapClaims{
			"sub": "user-123",

			"role": "customer",

			"scope": "products:read orders:write",

			"iss": "https://auth.yumzy.test",

			"aud": "yumzy-api",

			"exp": time.Now().
				Add(time.Hour).
				Unix(),
		},
		projectJWTTestSecret,
	)

	identity, err := verifier.ValidateIdentity(
		token,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		"user-123",
		identity.ClientID,
	)

	assert.Equal(
		t,
		"customer",
		identity.Role,
	)

	assert.Equal(
		t,
		[]string{
			"products:read",
			"orders:write",
		},
		identity.Scopes,
	)
}

func TestProjectJWTVerifierRejectsWrongSecret(
	t *testing.T,
) {
	verifier := testProjectJWTVerifier(t)

	token := signProjectJWTForTest(
		t,
		jwt.SigningMethodHS256,
		jwt.MapClaims{
			"sub": "user-123",

			"iss": "https://auth.yumzy.test",

			"aud": "yumzy-api",

			"exp": time.Now().
				Add(time.Hour).
				Unix(),
		},
		"abcdefghijklmnopqrstuvwxyz123456",
	)

	_, err := verifier.ValidateIdentity(
		token,
	)

	require.Error(t, err)
}

func TestProjectJWTVerifierRejectsExpired(
	t *testing.T,
) {
	verifier := testProjectJWTVerifier(t)

	token := signProjectJWTForTest(
		t,
		jwt.SigningMethodHS256,
		jwt.MapClaims{
			"sub": "user-123",

			"iss": "https://auth.yumzy.test",

			"aud": "yumzy-api",

			"exp": time.Now().
				Add(-time.Hour).
				Unix(),
		},
		projectJWTTestSecret,
	)

	_, err := verifier.ValidateIdentity(
		token,
	)

	require.Error(t, err)
}

func TestProjectJWTVerifierRequiresExpiration(
	t *testing.T,
) {
	verifier := testProjectJWTVerifier(t)

	token := signProjectJWTForTest(
		t,
		jwt.SigningMethodHS256,
		jwt.MapClaims{
			"sub": "user-123",

			"iss": "https://auth.yumzy.test",

			"aud": "yumzy-api",
		},
		projectJWTTestSecret,
	)

	_, err := verifier.ValidateIdentity(
		token,
	)

	require.Error(t, err)
}

func TestProjectJWTVerifierRejectsWrongIssuer(
	t *testing.T,
) {
	verifier := testProjectJWTVerifier(t)

	token := signProjectJWTForTest(
		t,
		jwt.SigningMethodHS256,
		jwt.MapClaims{
			"sub": "user-123",

			"iss": "https://wrong.example",

			"aud": "yumzy-api",

			"exp": time.Now().
				Add(time.Hour).
				Unix(),
		},
		projectJWTTestSecret,
	)

	_, err := verifier.ValidateIdentity(
		token,
	)

	require.Error(t, err)
}

func TestProjectJWTVerifierRejectsWrongAudience(
	t *testing.T,
) {
	verifier := testProjectJWTVerifier(t)

	token := signProjectJWTForTest(
		t,
		jwt.SigningMethodHS256,
		jwt.MapClaims{
			"sub": "user-123",

			"iss": "https://auth.yumzy.test",

			"aud": "wrong-api",

			"exp": time.Now().
				Add(time.Hour).
				Unix(),
		},
		projectJWTTestSecret,
	)

	_, err := verifier.ValidateIdentity(
		token,
	)

	require.Error(t, err)
}

func TestProjectJWTVerifierRejectsHS512(
	t *testing.T,
) {
	verifier := testProjectJWTVerifier(t)

	token := signProjectJWTForTest(
		t,
		jwt.SigningMethodHS512,
		jwt.MapClaims{
			"sub": "user-123",

			"iss": "https://auth.yumzy.test",

			"aud": "yumzy-api",

			"exp": time.Now().
				Add(time.Hour).
				Unix(),
		},
		projectJWTTestSecret,
	)

	_, err := verifier.ValidateIdentity(
		token,
	)

	require.Error(t, err)
}
