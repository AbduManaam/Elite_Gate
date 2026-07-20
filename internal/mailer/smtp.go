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
	msg := gomail.NewMsg()

	fromAddr := mail.Address{
		Name:    m.cfg.FromName,
		Address: m.cfg.FromEmail,
	}

	if err := msg.From(fromAddr.String()); err != nil {
		return fmt.Errorf("set from header: %w", err)
	}
	if err := msg.To(recipient); err != nil {
		return fmt.Errorf("set to header: %w", err)
	}

	msg.Subject("Reset your Elite Gateway password")

	body := fmt.Sprintf(
		"Hello,\n\nWe received a request to reset your Elite Gateway password.\n\nUse the link below to choose a new password:\n\n%s\n\nThis link expires in 15 minutes and can be used only once.\n\nIf you did not request this change, you can ignore this email.\n\nElite Gateway",
		resetURL,
	)
	msg.SetBodyString(gomail.TypeTextPlain, body)

	if err := m.client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}

	return nil
}
