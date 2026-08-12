package mailer

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	gomail "github.com/wneessen/go-mail"
)

type SMTPConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	FromEmail  string
	FromName   string
	TLSMode    string
	Production bool
}

var _ Mailer = (*SMTPMailer)(nil)

type SMTPMailer struct {
	cfg    SMTPConfig
	client *gomail.Client
}

func NewSMTPMailer(cfg SMTPConfig) (*SMTPMailer, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, fmt.Errorf("smtp host cannot be empty")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid smtp port: %d", cfg.Port)
	}
	if strings.TrimSpace(cfg.FromEmail) == "" {
		return nil, fmt.Errorf("smtp from_email cannot be empty")
	}
	if _, err := mail.ParseAddress(cfg.FromEmail); err != nil {
		return nil, fmt.Errorf("invalid smtp from_email format: %w", err)
	}
	if strings.TrimSpace(cfg.Username) != "" && strings.TrimSpace(cfg.Password) == "" {
		return nil, fmt.Errorf("smtp password is required when username is provided")
	}
	if cfg.Production && strings.EqualFold(cfg.TLSMode, "none") {
		return nil, fmt.Errorf("smtp TLS mode 'none' is not permitted in production")
	}

	opts := []gomail.Option{
		gomail.WithPort(cfg.Port),
	}

	switch strings.ToLower(strings.TrimSpace(cfg.TLSMode)) {
	case "starttls", "":
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	case "implicit":
		opts = append(opts, gomail.WithSSLPort(true))
	case "none":
		opts = append(opts, gomail.WithTLSPolicy(gomail.NoTLS))
	default:
		return nil, fmt.Errorf("unsupported tls_mode: %q", cfg.TLSMode)
	}

	if cfg.Username != "" {
		opts = append(opts, gomail.WithSMTPAuth(gomail.SMTPAuthPlain))
		opts = append(opts, gomail.WithUsername(cfg.Username))
		opts = append(opts, gomail.WithPassword(cfg.Password))
	}

	client, err := gomail.NewClient(cfg.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("create persistent smtp client: %w", err)
	}

	return &SMTPMailer{
		cfg:    cfg,
		client: client,
	}, nil
}

func (m *SMTPMailer) SendPasswordReset(ctx context.Context, recipient, resetURL string) error {
	msg := buildPasswordResetMsg(
		mail.Address{Name: m.cfg.FromName, Address: m.cfg.FromEmail},
		recipient,
		resetURL,
	)

	if err := m.client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}

	return nil
}

func (m *SMTPMailer) SendEmailVerification(ctx context.Context, recipient, verificationURL string) error {
	msg := buildEmailVerificationMsg(
		mail.Address{Name: m.cfg.FromName, Address: m.cfg.FromEmail},
		recipient,
		verificationURL,
	)

	if err := m.client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("send email verification email: %w", err)
	}

	return nil
}

func buildPasswordResetMsg(fromAddr mail.Address, recipient, resetURL string) *gomail.Msg {
	msg := gomail.NewMsg()
	_ = msg.From(fromAddr.String())
	_ = msg.To(recipient)
	msg.Subject("Reset your Elite Gateway password")

	body := fmt.Sprintf(
		"Hello,\n\nWe received a request to reset your Elite Gateway password.\n\nUse the link below to choose a new password:\n\n%s\n\nThis link expires in 15 minutes and can be used only once.\n\nIf you did not request this change, you can ignore this email.\n\nElite Gateway",
		resetURL,
	)
	msg.SetBodyString(gomail.TypeTextPlain, body)
	return msg
}

func buildEmailVerificationMsg(fromAddr mail.Address, recipient, verificationURL string) *gomail.Msg {
	msg := gomail.NewMsg()
	_ = msg.From(fromAddr.String())
	_ = msg.To(recipient)
	msg.Subject("Verify your Elite Gateway email")

	body := fmt.Sprintf(
		"Hello,\n\nThanks for creating an Elite Gateway account.\n\nPlease verify your email address using the link below:\n\n%s\n\nThis verification link expires in 30 minutes and can be used only once.\n\nIf you did not create this account, you can ignore this email.\n\nElite Gateway",
		verificationURL,
	)
	msg.SetBodyString(gomail.TypeTextPlain, body)
	return msg
}
