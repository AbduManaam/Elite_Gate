package password

import (
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errStr   string
	}{
		{
			name:     "valid password",
			password: "P@ssword123!",
			wantErr:  false,
		},
		{
			name:     "too short (7 characters)",
			password: "P@ssw1!",
			wantErr:  true,
			errStr:   "password must be at least 8 characters",
		},
		{
			name:     "exactly 8 characters - valid",
			password: "P@ss123!",
			wantErr:  false,
		},
		{
			name:     "too long (73 characters)",
			password: "P@ssword123!P@ssword123!P@ssword123!P@ssword123!P@ssword123!P@ssword123!P@ss",
			wantErr:  true,
			errStr:   "password must not exceed 72 characters",
		},
		{
			name:     "weak password from blocklist",
			password: "admin123",
			wantErr:  true,
			errStr:   `password "admin123" is too common — choose a stronger one`,
		},
		{
			name:     "missing uppercase",
			password: "p@ssword123!",
			wantErr:  true,
			errStr:   "password must contain at least one uppercase letter (A-Z)",
		},
		{
			name:     "missing lowercase",
			password: "P@SSWORD123!",
			wantErr:  true,
			errStr:   "password must contain at least one lowercase letter (a-z)",
		},
		{
			name:     "missing digit",
			password: "P@ssword!!!!",
			wantErr:  true,
			errStr:   "password must contain at least one digit (0-9)",
		},
		{
			name:     "missing special",
			password: "Password1234",
			wantErr:  true,
			errStr:   "password must contain at least one special character (!@#$%^&* etc.)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.password)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && err.Error() != tt.errStr {
				t.Fatalf("Validate() error message = %q, want %q", err.Error(), tt.errStr)
			}
		})
	}
}
