package helper

import (
	"errors"
	"testing"
)

func TestNormalizeHostname(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "valid subdomain",
			input:    "api.acmecorp.com",
			expected: "api.acmecorp.com",
		},
		{
			name:     "normalizes uppercase and trailing dot",
			input:    "API.AcmeCorp.COM.",
			expected: "api.acmecorp.com",
		},
		{
			name:     "removes surrounding spaces",
			input:    "  api.acmecorp.com  ",
			expected: "api.acmecorp.com",
		},
		{
			name:     "accepts hyphen inside label",
			input:    "customer-api.acmecorp.com",
			expected: "customer-api.acmecorp.com",
		},
		{
			name:     "accepts multi-level public suffix",
			input:    "api.acmecorp.co.uk",
			expected: "api.acmecorp.co.uk",
		},
		{
			name:    "rejects empty hostname",
			input:   "",
			wantErr: true,
		},
		{
			name:    "rejects URL scheme",
			input:   "https://api.acmecorp.com",
			wantErr: true,
		},
		{
			name:    "rejects path",
			input:   "api.acmecorp.com/users",
			wantErr: true,
		},
		{
			name:    "rejects query string",
			input:   "api.acmecorp.com?test=true",
			wantErr: true,
		},
		{
			name:    "rejects port",
			input:   "api.acmecorp.com:8080",
			wantErr: true,
		},
		{
			name:    "rejects localhost",
			input:   "localhost",
			wantErr: true,
		},
		{
			name:    "rejects IPv4 address",
			input:   "192.168.1.10",
			wantErr: true,
		},
		{
			name:    "rejects IPv6 address",
			input:   "::1",
			wantErr: true,
		},
		{
			name:    "rejects wildcard",
			input:   "*.acmecorp.com",
			wantErr: true,
		},
		{
			name:    "rejects apex domain for MVP",
			input:   "acmecorp.com",
			wantErr: true,
		},
		{
			name:    "rejects label beginning with hyphen",
			input:   "-api.acmecorp.com",
			wantErr: true,
		},
		{
			name:    "rejects label ending with hyphen",
			input:   "api-.acmecorp.com",
			wantErr: true,
		},
		{
			name:    "rejects consecutive dots",
			input:   "api..acmecorp.com",
			wantErr: true,
		},
		{
			name:    "rejects underscore",
			input:   "api_test.acmecorp.com",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := NormalizeHostname(test.input)

			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil with result %q", result)
				}

				if !errors.Is(err, ErrInvalidHostname) {
					t.Fatalf(
						"expected ErrInvalidHostname, got %v",
						err,
					)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != test.expected {
				t.Fatalf(
					"expected %q, got %q",
					test.expected,
					result,
				)
			}
		})
	}
}
