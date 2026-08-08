package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"elitegate/internal/auth"
	"elitegate/internal/model"
	"elitegate/internal/shared"

	"github.com/stretchr/testify/assert"
)

type fakeIdentityValidator struct {
	identity *auth.Identity
	err      error
}

func (f *fakeIdentityValidator) ValidateIdentity(
	_ string,
) (*auth.Identity, error) {
	return f.identity, f.err
}

func TestAuthMiddlewareUsesJWTScopes(
	t *testing.T,
) {
	validator := &fakeIdentityValidator{
		identity: &auth.Identity{
			ClientID: "user-1",
			Role:     "customer",

			Scopes: []string{
				"products:read",
			},
		},
	}

	mw := NewAuthMiddleware(
		validator,
		nil,
		nil,
	)

	handler := mw.Middleware(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.WriteHeader(
					http.StatusOK,
				)
			},
		),
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/products",
		nil,
	)

	req.Header.Set(
		"Authorization",
		"Bearer test-token",
	)

	route := &model.Route{
		AuthRequired: true,

		AllowedRoles: []string{
			"customer",
		},

		AllowedScopes: []string{
			"products:read",
		},
	}

	ctx := context.WithValue(
		req.Context(),
		shared.ContextKeyRoute,
		route,
	)

	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.ServeHTTP(
		w,
		req,
	)

	assert.Equal(
		t,
		http.StatusOK,
		w.Code,
	)
}
