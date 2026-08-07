package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrProjectJWTNotConfigured = errors.New(
	"project JWT authentication is not configured",
)

type Identity struct {
	ClientID string
	Role     string
	Scopes   []string
}

type ProjectJWTVerifierConfig struct {
	Algorithm string

	Issuer    *string
	Audiences []string

	SubjectClaim string
	RoleClaim    string
	ScopesClaim  string

	ClockSkew time.Duration
}

type ProjectJWTVerifier struct {
	secret []byte
	config ProjectJWTVerifierConfig
}

func NewProjectJWTVerifier(
	secret string,
	cfg ProjectJWTVerifierConfig,
) (*ProjectJWTVerifier, error) {
	if len([]byte(secret)) < 32 {
		return nil, errors.New(
			"HS256 verification secret must be at least 32 bytes",
		)
	}

	if cfg.Algorithm != "HS256" {
		return nil, fmt.Errorf(
			"unsupported JWT algorithm %q",
			cfg.Algorithm,
		)
	}

	if cfg.SubjectClaim == "" {
		cfg.SubjectClaim = "sub"
	}

	if cfg.RoleClaim == "" {
		cfg.RoleClaim = "role"
	}

	if cfg.ScopesClaim == "" {
		cfg.ScopesClaim = "scope"
	}

	return &ProjectJWTVerifier{
		secret: []byte(secret),
		config: cfg,
	}, nil
}

func (v *ProjectJWTVerifier) Validate(
	tokenString string,
) (*Identity, error) {
	if v == nil {
		return nil, ErrProjectJWTNotConfigured
	}

	tokenString = strings.TrimSpace(tokenString)

	if tokenString == "" {
		return nil, errors.New("JWT is empty")
	}

	claims := jwt.MapClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != v.config.Algorithm {
				return nil, fmt.Errorf(
					"unexpected JWT algorithm %q",
					token.Method.Alg(),
				)
			}

			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New(
					"JWT signing method is not HMAC",
				)
			}

			return v.secret, nil
		},
		jwt.WithValidMethods(
			[]string{v.config.Algorithm},
		),
		jwt.WithLeeway(v.config.ClockSkew),
	)

	if err != nil {
		return nil, fmt.Errorf(
			"validate JWT: %w",
			err,
		)
	}

	if token == nil || !token.Valid {
		return nil, errors.New("JWT is invalid")
	}

	if err := v.validateIssuer(claims); err != nil {
		return nil, err
	}

	if err := v.validateAudience(claims); err != nil {
		return nil, err
	}

	clientID, err := requiredStringClaim(
		claims,
		v.config.SubjectClaim,
	)
	if err != nil {
		return nil, err
	}

	role := optionalStringClaim(
		claims,
		v.config.RoleClaim,
	)

	scopes, err := scopesClaim(
		claims,
		v.config.ScopesClaim,
	)
	if err != nil {
		return nil, err
	}

	return &Identity{
		ClientID: clientID,
		Role:     role,
		Scopes:   scopes,
	}, nil
}

func (v *ProjectJWTVerifier) validateIssuer(
	claims jwt.MapClaims,
) error {
	if v.config.Issuer == nil {
		return nil
	}

	actual, ok := claims["iss"].(string)

	if !ok || actual != *v.config.Issuer {
		return errors.New("JWT issuer does not match")
	}

	return nil
}

func (v *ProjectJWTVerifier) validateAudience(
	claims jwt.MapClaims,
) error {
	if len(v.config.Audiences) == 0 {
		return nil
	}

	actual := stringListClaim(
		claims["aud"],
	)

	for _, expected := range v.config.Audiences {
		for _, candidate := range actual {
			if candidate == expected {
				return nil
			}
		}
	}

	return errors.New("JWT audience does not match")
}

func requiredStringClaim(
	claims jwt.MapClaims,
	name string,
) (string, error) {
	value := optionalStringClaim(
		claims,
		name,
	)

	if value == "" {
		return "", fmt.Errorf(
			"required JWT claim %q is missing",
			name,
		)
	}

	return value, nil
}

func optionalStringClaim(
	claims jwt.MapClaims,
	name string,
) string {
	value, ok := claims[name]
	if !ok {
		return ""
	}

	str, ok := value.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(str)
}

func scopesClaim(
	claims jwt.MapClaims,
	name string,
) ([]string, error) {
	value, ok := claims[name]
	if !ok {
		return []string{}, nil
	}

	switch typed := value.(type) {
	case string:
		return uniqueStrings(
			strings.Fields(typed),
		), nil

	case []string:
		return uniqueStrings(typed), nil

	case []any:
		values := make(
			[]string,
			0,
			len(typed),
		)

		for _, item := range typed {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf(
					"JWT scope claim %q contains a non-string value",
					name,
				)
			}

			values = append(values, str)
		}

		return uniqueStrings(values), nil

	default:
		return nil, fmt.Errorf(
			"JWT scope claim %q has unsupported format",
			name,
		)
	}
}

func stringListClaim(
	value any,
) []string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}

		return []string{typed}

	case []string:
		return typed

	case []any:
		result := make(
			[]string,
			0,
			len(typed),
		)

		for _, item := range typed {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}

		return result
	}

	return nil
}

func uniqueStrings(
	values []string,
) []string {
	seen := make(
		map[string]struct{},
		len(values),
	)

	result := make(
		[]string,
		0,
		len(values),
	)

	for _, value := range values {
		value = strings.TrimSpace(value)

		if value == "" {
			continue
		}

		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}
