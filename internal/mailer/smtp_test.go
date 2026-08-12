package mailer

import (
	"bytes"
	"context"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestSMTPMailer_InterfaceAssertion(t *testing.T) {
	var _ Mailer = (*SMTPMailer)(nil)
}

func TestBuildEmailVerificationMsg(t *testing.T) {
	from := mail.Address{Name: "Elite Gateway", Address: "noreply@elitegateway.site"}
	recipient := "user@example.com"
	verificationURL := "https://elitegateway.site/verify-email?token=sample_token_123"

	msg := buildEmailVerificationMsg(from, recipient, verificationURL)

	buf := new(bytes.Buffer)
	if _, err := msg.WriteTo(buf); err != nil {
		t.Fatalf("failed to write message to buffer: %v", err)
	}

	rawMsg := buf.String()

	if !strings.Contains(rawMsg, "Subject: Verify your Elite Gateway email") {
		t.Errorf("expected subject 'Subject: Verify your Elite Gateway email' in raw message, got message:\n%s", rawMsg)
	}

	// gomail QP encodes '=' as '=3D'
	expectedQPURL := strings.ReplaceAll(verificationURL, "=", "=3D")
	if !strings.Contains(rawMsg, verificationURL) && !strings.Contains(rawMsg, expectedQPURL) {
		t.Errorf("expected email body to contain verification URL %q or QP encoded %q", verificationURL, expectedQPURL)
	}

	if !strings.Contains(rawMsg, "This verification link expires in 30 minutes and can be used only once.") {
		t.Error("expected email body to contain 30-minute expiry explanation")
	}

	if !strings.Contains(rawMsg, "Thanks for creating an Elite Gateway account.") {
		t.Error("expected email body to contain welcome text")
	}
}

func TestBuildPasswordResetMsg(t *testing.T) {
	from := mail.Address{Name: "Elite Gateway", Address: "noreply@elitegateway.site"}
	recipient := "user@example.com"
	resetURL := "https://elitegateway.site/reset-password?token=reset_token_456"

	msg := buildPasswordResetMsg(from, recipient, resetURL)

	buf := new(bytes.Buffer)
	if _, err := msg.WriteTo(buf); err != nil {
		t.Fatalf("failed to write message to buffer: %v", err)
	}

	rawMsg := buf.String()

	if !strings.Contains(rawMsg, "Subject: Reset your Elite Gateway password") {
		t.Errorf("expected subject 'Subject: Reset your Elite Gateway password' in raw message, got message:\n%s", rawMsg)
	}

	expectedQPURL := strings.ReplaceAll(resetURL, "=", "=3D")
	if !strings.Contains(rawMsg, resetURL) && !strings.Contains(rawMsg, expectedQPURL) {
		t.Errorf("expected email body to contain reset URL %q or QP encoded %q", resetURL, expectedQPURL)
	}

	if !strings.Contains(rawMsg, "This link expires in 15 minutes and can be used only once.") {
		t.Error("expected email body to contain 15-minute expiry explanation")
	}
}

func TestSMTPMailer_NewSMTPMailer_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SMTPConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: SMTPConfig{
				Host:      "smtp.example.com",
				Port:      587,
				FromEmail: "noreply@example.com",
				TLSMode:   "starttls",
			},
			wantErr: false,
		},
		{
			name: "empty host",
			cfg: SMTPConfig{
				Host:      "",
				Port:      587,
				FromEmail: "noreply@example.com",
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			cfg: SMTPConfig{
				Host:      "smtp.example.com",
				Port:      0,
				FromEmail: "noreply@example.com",
			},
			wantErr: true,
		},
		{
			name: "missing from email",
			cfg: SMTPConfig{
				Host:      "smtp.example.com",
				Port:      587,
				FromEmail: "",
			},
			wantErr: true,
		},
		{
			name: "none TLS in production",
			cfg: SMTPConfig{
				Host:       "smtp.example.com",
				Port:       587,
				FromEmail:  "noreply@example.com",
				TLSMode:    "none",
				Production: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSMTPMailer(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSMTPMailer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSMTPMailer_ContextCancellation(t *testing.T) {
	mailer, err := NewSMTPMailer(SMTPConfig{
		Host:      "127.0.0.1",
		Port:      587,
		FromEmail: "noreply@example.com",
		TLSMode:   "none",
	})
	if err != nil {
		t.Fatalf("failed to create mailer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = mailer.SendEmailVerification(ctx, "user@example.com", "https://example.com/verify")
	if err == nil {
		t.Error("expected error when context is canceled, got nil")
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel2()
	time.Sleep(2 * time.Millisecond)

	err = mailer.SendPasswordReset(ctx2, "user@example.com", "https://example.com/reset")
	if err == nil {
		t.Error("expected error when context is timed out, got nil")
	}
}
