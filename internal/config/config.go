package config

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

type LogConfig struct {
	Level      string `mapstructure:"level"`
	File       string `mapstructure:"file"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
}

type GoogleOAuthConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
	StateSecret  string `mapstructure:"state_secret"`
	FrontendURL  string `mapstructure:"frontend_url"`
}

type ServerConfig struct {
	Port                string            `mapstructure:"port"`
	GatewayPort         string            `mapstructure:"gateway_port"`
	AdminPort           string            `mapstructure:"admin_port"`
	AdminAPIURL         string            `mapstructure:"admin_api_url"`
	GatewayImageName    string            `mapstructure:"gateway_image_name"`
	ReadTimeout         string            `mapstructure:"read_timeout"`
	WriteTimeout        string            `mapstructure:"write_timeout"`
	IdleTimeout         string            `mapstructure:"idle_timeout"`
	ShutdownTimeout     string            `mapstructure:"shutdown_timeout"`
	DrainTimeout        string            `mapstructure:"drain_timeout"`
	DrainStaleAfter     string            `mapstructure:"drain_stale_after"`
	GRPCGatewayPort     string            `mapstructure:"grpc_gateway_port"`
	RouteReloadInterval string            `mapstructure:"route_reload_interval"`
	ProjectID           string            `mapstructure:"project_id"`
	DevHostMap          map[string]string `mapstructure:"dev_host_map"`
	AdminIPAllowlist    []string          `mapstructure:"admin_ip_allowlist"`
	TrustProxy          bool              `mapstructure:"trust_proxy"`
	TrustProxyHeaders   bool              `mapstructure:"trust_proxy_headers"`
	AllowedOrigins      []string          `mapstructure:"allowed_origins"`
	PrometheusURL       string            `mapstructure:"prometheus_url"`
	MetricsCacheTTL     string            `mapstructure:"metrics_cache_ttl"`
}

type DatabaseConfig struct {
	DSN          string `mapstructure:"dsn"`
	GatewayDSN   string `mapstructure:"gateway_dsn"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type AuthConfig struct {
	JWTSecret        string `mapstructure:"jwt_secret"`
	// GatewaySyncToken is a pre-derived HMAC token used by gateway containers
	// to authenticate against the control-plane /internal/v1/projects/:id/sync
	// endpoint. If empty, the gateway derives it at startup from JWTSecret + ProjectID.
	GatewaySyncToken string `mapstructure:"gateway_sync_token"`
}

type MailConfig struct {
	Enabled          bool   `mapstructure:"enabled"`
	Host             string `mapstructure:"host"`
	Port             int    `mapstructure:"port"`
	Username         string `mapstructure:"username"`
	Password         string `mapstructure:"password"`
	FromEmail        string `mapstructure:"from_email"`
	FromName         string `mapstructure:"from_name"`
	PasswordResetURL string `mapstructure:"password_reset_url"`
	TLSMode          string `mapstructure:"tls_mode"` // "starttls", "implicit", "none"
}

type AuthRateLimitConfig struct {
	LoginRPM          int `mapstructure:"login_rpm"`
	RefreshRPM        int `mapstructure:"refresh_rpm"`
	OAuthCallbackRPM  int `mapstructure:"oauth_callback_rpm"`
	SignupRPM         int `mapstructure:"signup_rpm"`
	ForgotPasswordRPM int `mapstructure:"forgot_password_rpm"`
	ResetPasswordRPM  int `mapstructure:"reset_password_rpm"`
}

type RateLimitConfig struct {
	RequestsPerMinute int                 `mapstructure:"requests_per_minute"`
	Auth              AuthRateLimitConfig `mapstructure:"auth"`
}

type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Log         LogConfig         `mapstructure:"log"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Redis       RedisConfig       `mapstructure:"redis"`
	Auth        AuthConfig        `mapstructure:"auth"`
	GoogleOAuth GoogleOAuthConfig `mapstructure:"google_oauth"`
	Mail        MailConfig        `mapstructure:"mail"`
	RateLimit   RateLimitConfig   `mapstructure:"rate_limit"`
	AppEnv      string            `mapstructure:"app_env"`
}

func LoadConfig() (*Config, error) {
	// 1. Load .env file variables into runtime env if it exists
	_ = gotenv.Load()

	// 2. Setup Viper and config defaults
	viper.SetConfigFile("internal/config/config.yaml")

	viper.SetDefault("server.port", ":8080")
	viper.SetDefault("server.admin_api_url", "http://admin:9090")
	viper.SetDefault("server.prometheus_url", "http://prometheus:9090")
	viper.SetDefault("server.metrics_cache_ttl", "30s")
	viper.SetDefault("database.max_open_conns", 25)
	viper.SetDefault("database.max_idle_conns", 5)
	viper.SetDefault("redis.addr", "redis:6379")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("server.read_timeout", "15s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.idle_timeout", "60s")
	viper.SetDefault("server.shutdown_timeout", "30s")
	viper.SetDefault("server.drain_timeout", "5s")
	viper.SetDefault("server.drain_stale_after", "2m")
	viper.SetDefault("server.grpc_gateway_port", ":50051")
	viper.SetDefault("server.route_reload_interval", "10s")
	viper.SetDefault("server.trust_proxy_headers", false)
	viper.SetDefault("rate_limit.requests_per_minute", 100)
	viper.SetDefault("rate_limit.auth.login_rpm", 15)
	viper.SetDefault("rate_limit.auth.refresh_rpm", 30)
	viper.SetDefault("rate_limit.auth.oauth_callback_rpm", 10)
	viper.SetDefault("rate_limit.auth.signup_rpm", 5)
	viper.SetDefault("rate_limit.auth.forgot_password_rpm", 5)
	viper.SetDefault("rate_limit.auth.reset_password_rpm", 10)
	viper.SetDefault("mail.enabled", false)
	viper.SetDefault("mail.port", 587)
	viper.SetDefault("mail.from_name", "Elite Gateway")
	viper.SetDefault("mail.tls_mode", "starttls")
	viper.SetDefault("mail.password_reset_url", "http://localhost:5173/reset-password")
	viper.SetDefault("app_env", "development")

	// 3. Read the YAML configuration file if it exists
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// 4. Bind env overrides (takes precedence over yaml configs)
	viper.BindEnv("database.dsn", "POSTGRES_DSN")
	viper.BindEnv("database.gateway_dsn", "POSTGRES_GATEWAY_DSN")
	viper.BindEnv("redis.addr", "REDIS_ADDR")
	viper.BindEnv("redis.password", "REDIS_PASSWORD")
	viper.BindEnv("server.gateway_port", "GATEWAY_PORT")
	viper.BindEnv("server.admin_port", "ADMIN_PORT")
	viper.BindEnv("server.admin_api_url", "ADMIN_API_URL")
	viper.BindEnv("server.gateway_image_name", "GATEWAY_IMAGE_NAME")
	viper.BindEnv("auth.jwt_secret", "JWT_SECRET")
	viper.BindEnv("auth.gateway_sync_token", "GATEWAY_SYNC_TOKEN")
	viper.BindEnv("rate_limit.requests_per_minute", "RATE_LIMIT_RPM")
	viper.BindEnv("server.read_timeout", "SERVER_READ_TIMEOUT")
	viper.BindEnv("server.write_timeout", "SERVER_WRITE_TIMEOUT")
	viper.BindEnv("server.idle_timeout", "SERVER_IDLE_TIMEOUT")
	viper.BindEnv("server.shutdown_timeout", "SERVER_SHUTDOWN_TIMEOUT")
	viper.BindEnv("server.drain_timeout", "SERVER_DRAIN_TIMEOUT")
	viper.BindEnv("server.drain_stale_after", "SERVER_DRAIN_STALE_AFTER")
	viper.BindEnv("server.grpc_gateway_port", "GRPC_GATEWAY_PORT")
	viper.BindEnv("server.route_reload_interval", "ROUTE_RELOAD_INTERVAL")
	viper.BindEnv("app_env", "APP_ENV")
	viper.BindEnv("server.project_id", "PROJECT_ID")
	viper.BindEnv("server.trust_proxy", "TRUST_PROXY")
	viper.BindEnv("server.trust_proxy_headers", "TRUST_PROXY_HEADERS")
	viper.BindEnv("rate_limit.auth.login_rpm", "RATE_LIMIT_AUTH_LOGIN_RPM")
	viper.BindEnv("rate_limit.auth.refresh_rpm", "RATE_LIMIT_AUTH_REFRESH_RPM")
	viper.BindEnv("rate_limit.auth.oauth_callback_rpm", "RATE_LIMIT_AUTH_OAUTH_CALLBACK_RPM")
	viper.BindEnv("rate_limit.auth.signup_rpm", "RATE_LIMIT_AUTH_SIGNUP_RPM")
	viper.BindEnv("rate_limit.auth.forgot_password_rpm", "RATE_LIMIT_AUTH_FORGOT_PASSWORD_RPM")
	viper.BindEnv("rate_limit.auth.reset_password_rpm", "RATE_LIMIT_AUTH_RESET_PASSWORD_RPM")
	viper.BindEnv("server.prometheus_url", "PROMETHEUS_URL")
	viper.BindEnv("server.metrics_cache_ttl", "METRICS_CACHE_TTL")
	viper.BindEnv("google_oauth.client_id", "GOOGLE_CLIENT_ID")
	viper.BindEnv("google_oauth.client_secret", "GOOGLE_CLIENT_SECRET")
	viper.BindEnv("google_oauth.redirect_url", "GOOGLE_REDIRECT_URL")
	viper.BindEnv("google_oauth.state_secret", "OAUTH_STATE_SECRET")
	viper.BindEnv("google_oauth.frontend_url", "FRONTEND_URL")
	viper.BindEnv("mail.enabled", "SMTP_ENABLED")
	viper.BindEnv("mail.host", "SMTP_HOST")
	viper.BindEnv("mail.port", "SMTP_PORT")
	viper.BindEnv("mail.username", "SMTP_USERNAME")
	viper.BindEnv("mail.password", "SMTP_PASSWORD")
	viper.BindEnv("mail.from_email", "SMTP_FROM_EMAIL")
	viper.BindEnv("mail.from_name", "SMTP_FROM_NAME")
	viper.BindEnv("mail.tls_mode", "SMTP_TLS_MODE")
	viper.BindEnv("mail.password_reset_url", "PASSWORD_RESET_URL")

	viper.AutomaticEnv()

	var cfg Config

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if allowlistRaw := os.Getenv("ADMIN_IP_ALLOWLIST"); allowlistRaw != "" {
		cfg.Server.AdminIPAllowlist = strings.Split(allowlistRaw, ",")
		for i, v := range cfg.Server.AdminIPAllowlist {
			cfg.Server.AdminIPAllowlist[i] = strings.TrimSpace(v)
		}
	}

	if hostMapRaw := os.Getenv("GATEWAY_HOST_MAP"); hostMapRaw != "" {
		cfg.Server.DevHostMap = parseHostMap(hostMapRaw)
	}

	// ALLOWED_ORIGINS env var, if set, fully replaces whatever config.yaml has.
	// Placed after Unmarshal since AllowedOrigins isn't a registered viper key.
	if allowedOriginsRaw := os.Getenv("ALLOWED_ORIGINS"); allowedOriginsRaw != "" {
		cfg.Server.AllowedOrigins = strings.Split(allowedOriginsRaw, ",")
		for i, v := range cfg.Server.AllowedOrigins {
			cfg.Server.AllowedOrigins[i] = strings.TrimSpace(v)
		}
	}

	// 5. Validation Check
	// POSTGRES_DSN is intentionally optional here: the data-plane gateway no
	// longer talks to Postgres (it syncs from the control plane). Admin and
	// worker validate DSN themselves before opening a DB connection.
	if cfg.Auth.JWTSecret == "" {
		return nil, errors.New("JWT secret (JWT_SECRET) is required")
	}
	if cfg.GoogleOAuth.ClientID != "" {
		if cfg.GoogleOAuth.ClientSecret == "" ||
			cfg.GoogleOAuth.RedirectURL == "" ||
			cfg.GoogleOAuth.StateSecret == "" {
			return nil, errors.New("GOOGLE_CLIENT_ID is set but GOOGLE_CLIENT_SECRET/GOOGLE_REDIRECT_URL/OAUTH_STATE_SECRET are missing")
		}

		if len(cfg.GoogleOAuth.StateSecret) < 32 {
			return nil, errors.New("OAUTH_STATE_SECRET must be at least 32 bytes")
		}
	}
	if cfg.RateLimit.RequestsPerMinute <= 0 {
		return nil, errors.New("rate_limit.requests_per_minute must be > 0")
	}
	if len(cfg.Server.AllowedOrigins) == 0 {
		return nil, errors.New("server.allowed_origins must have at least one entry")
	}
	if err := validateDuration("server.read_timeout", cfg.Server.ReadTimeout); err != nil {
		return nil, err
	}
	if err := validateDuration("server.write_timeout", cfg.Server.WriteTimeout); err != nil {
		return nil, err
	}
	if err := validateDuration("server.idle_timeout", cfg.Server.IdleTimeout); err != nil {
		return nil, err
	}
	if err := validateDuration("server.shutdown_timeout", cfg.Server.ShutdownTimeout); err != nil {
		return nil, err
	}
	if err := validateDuration("server.drain_timeout", cfg.Server.DrainTimeout); err != nil {
		return nil, err
	}
	if err := validateDuration("server.drain_stale_after", cfg.Server.DrainStaleAfter); err != nil {
		return nil, err
	}
	if err := validateDuration("server.route_reload_interval", cfg.Server.RouteReloadInterval); err != nil {
		return nil, err
	}
	if err := validateMailConfig(cfg.Mail, cfg.AppEnv == "production"); err != nil {
		return nil, fmt.Errorf("mail configuration error: %w", err)
	}

	return &cfg, nil
}

func validatePasswordResetURL(rawURL string, production bool) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid password reset URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("password reset URL must use http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("password reset URL must contain a host")
	}
	if production && parsed.Scheme != "https" {
		return fmt.Errorf("password reset URL must use HTTPS in production")
	}
	return nil
}

func validateMailConfig(cfg MailConfig, production bool) error {
	if !cfg.Enabled {
		if production {
			return errors.New("mail must be enabled (SMTP_ENABLED=true) in production")
		}
		return nil
	}

	if strings.TrimSpace(cfg.Host) == "" {
		return errors.New("smtp.host cannot be empty when mail is enabled")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("invalid smtp.port: %d", cfg.Port)
	}

	fromEmail := strings.TrimSpace(cfg.FromEmail)
	if fromEmail == "" {
		return errors.New("smtp.from_email cannot be empty when mail is enabled")
	}
	if _, err := mail.ParseAddress(fromEmail); err != nil {
		return fmt.Errorf("invalid smtp.from_email format: %w", err)
	}

	username := strings.TrimSpace(cfg.Username)
	password := strings.TrimSpace(cfg.Password)
	if username != "" && password == "" {
		return errors.New("smtp.password is required when username is provided")
	}

	tlsMode := strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	switch tlsMode {
	case "", "starttls", "implicit":
		// allowed modes
	case "none":
		if production {
			return errors.New("smtp.tls_mode 'none' is not permitted in production")
		}
	default:
		return fmt.Errorf("unsupported smtp.tls_mode: %q", cfg.TLSMode)
	}

	return validatePasswordResetURL(cfg.PasswordResetURL, production)
}

func validateDuration(name, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%s cannot be empty", name)
	}
	if _, err := time.ParseDuration(raw); err != nil {
		return fmt.Errorf("%s has invalid duration %q: %w", name, raw, err)
	}
	return nil
}

func parseHostMap(raw string) map[string]string {
	out := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key != "" && value != "" {
			out[key] = value
		}
	}
	return out
}
