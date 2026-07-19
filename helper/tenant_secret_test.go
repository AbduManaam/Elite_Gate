package helper_test

import (
	"testing"

	"elitegate/helper"
	"elitegate/internal/auth"

	"github.com/golang-jwt/jwt/v5"
)

func TestDeriveTenantJWTSecret(t *testing.T) {
	masterSecret := "super-secret-master-key-that-is-at-least-32-bytes-long!!"
	projectID1 := "proj_11111111-1111-1111-1111-111111111111"
	projectID2 := "proj_22222222-2222-2222-2222-222222222222"

	t.Run("stable and deterministic", func(t *testing.T) {
		s1 := helper.DeriveTenantJWTSecret(masterSecret, projectID1)
		s2 := helper.DeriveTenantJWTSecret(masterSecret, projectID1)
		if s1 != s2 {
			t.Errorf("expected deterministic secret output, got %s and %s", s1, s2)
		}
	})

	t.Run("unique per project", func(t *testing.T) {
		s1 := helper.DeriveTenantJWTSecret(masterSecret, projectID1)
		s2 := helper.DeriveTenantJWTSecret(masterSecret, projectID2)
		if s1 == s2 {
			t.Errorf("expected different secrets for different projects, got identical: %s", s1)
		}
	})

	t.Run("token signed with derived secret is rejected by AdminTokenManager", func(t *testing.T) {
		mgr, err := auth.NewAdminTokenManager(masterSecret, "elitegate-admin")
		if err != nil {
			t.Fatalf("failed to create AdminTokenManager: %v", err)
		}

		tenantSecret := helper.DeriveTenantJWTSecret(masterSecret, projectID1)

		// Create token signed with tenant secret
		claims := auth.AdminClaims{
			Username: "fake_admin",
			Role:     auth.AdminRole,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "usr_fake123",
				Issuer:  "elitegate-admin",
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signedToken, err := token.SignedString([]byte(tenantSecret))
		if err != nil {
			t.Fatalf("failed to sign token with tenant secret: %v", err)
		}

		// Validate using master secret via AdminTokenManager -> MUST FAIL
		_, err = mgr.ValidateAdminAccessToken(signedToken)
		if err == nil {
			t.Fatal("CRITICAL SECURITY RISK: AdminTokenManager accepted token signed with derived tenant secret!")
		}
	})
}
