package config

import (
	"errors"
	"fmt"
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
	AllowedOrigins      []string          `mapstructure:"allowed_origins"`
	PrometheusURL       string            `mapstructure:"prometheus_url"`
	MetricsCacheTTL     string            `mapstructure:"metrics_cache_ttl"`
}

type DatabaseConfig struct {
	DSN          string `mapstructure:"dsn"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type AuthConfig struct {
	JWTSecret string `mapstructure:"jwt_secret"`
}

type RateLimitConfig struct {
	RequestsPerMinute int `mapstructure:"requests_per_minute"`
}

type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Log         LogConfig         `mapstructure:"log"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Redis       RedisConfig       `mapstructure:"redis"`
	Auth        AuthConfig        `mapstructure:"auth"`
	GoogleOAuth GoogleOAuthConfig `mapstructure:"google_oauth"`
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
	viper.SetDefault("rate_limit.requests_per_minute", 100)
	viper.SetDefault("app_env", "development")

	// 3. Read the YAML configuration file if it exists
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// 4. Bind env overrides (takes precedence over yaml configs)
	viper.BindEnv("database.dsn", "POSTGRES_DSN")
	viper.BindEnv("redis.addr", "REDIS_ADDR")
	viper.BindEnv("redis.password", "REDIS_PASSWORD")
	viper.BindEnv("server.gateway_port", "GATEWAY_PORT")
	viper.BindEnv("server.admin_port", "ADMIN_PORT")
	viper.BindEnv("server.admin_api_url", "ADMIN_API_URL")
	viper.BindEnv("auth.jwt_secret", "JWT_SECRET")
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
	viper.BindEnv("server.prometheus_url", "PROMETHEUS_URL")
	viper.BindEnv("server.metrics_cache_ttl", "METRICS_CACHE_TTL")
	viper.BindEnv("google_oauth.client_id", "GOOGLE_CLIENT_ID")
	viper.BindEnv("google_oauth.client_secret", "GOOGLE_CLIENT_SECRET")
	viper.BindEnv("google_oauth.redirect_url", "GOOGLE_REDIRECT_URL")
	viper.BindEnv("google_oauth.state_secret", "OAUTH_STATE_SECRET")
	viper.BindEnv("google_oauth.frontend_url", "FRONTEND_URL")

	viper.AutomaticEnv()

	var cfg Config

	if allowlistRaw := os.Getenv("ADMIN_IP_ALLOWLIST"); allowlistRaw != "" {
		cfg.Server.AdminIPAllowlist = strings.Split(allowlistRaw, ",")
		for i, v := range cfg.Server.AdminIPAllowlist {
			cfg.Server.AdminIPAllowlist[i] = strings.TrimSpace(v)
		}
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
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
	if cfg.Database.DSN == "" {
		return nil, errors.New("database connection DSN (POSTGRES_DSN) is required")
	}
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

	return &cfg, nil
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
