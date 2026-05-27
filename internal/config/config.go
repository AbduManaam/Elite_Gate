package config

import (
	"errors"
	"fmt"
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

type ServerConfig struct {
	Port            string `mapstructure:"port"`
	GatewayPort     string `mapstructure:"gateway_port"`
	AdminPort       string `mapstructure:"admin_port"`
	AdminAPIURL     string `mapstructure:"admin_api_url"`
	ReadTimeout     string `mapstructure:"read_timeout"`
	WriteTimeout    string `mapstructure:"write_timeout"`
	IdleTimeout     string `mapstructure:"idle_timeout"`
	ShutdownTimeout string `mapstructure:"shutdown_timeout"`
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
	Server    ServerConfig    `mapstructure:"server"`
	Log       LogConfig       `mapstructure:"log"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Auth      AuthConfig      `mapstructure:"auth"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	AppEnv    string          `mapstructure:"app_env"`
}

func LoadConfig() (*Config, error) {
	// 1. Load .env file variables into runtime env if it exists
	_ = gotenv.Load()

	// 2. Setup Viper and config defaults
	viper.SetConfigFile("internal/config/config.yaml")

	viper.SetDefault("server.port", ":8080")
	viper.SetDefault("server.admin_api_url", "http://admin:9090")
	viper.SetDefault("database.max_open_conns", 25)
	viper.SetDefault("database.max_idle_conns", 5)
	viper.SetDefault("redis.addr", "redis:6379")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("server.read_timeout", "15s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.idle_timeout", "60s")
	viper.SetDefault("server.shutdown_timeout", "30s")
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
	viper.BindEnv("app_env", "APP_ENV")

	viper.AutomaticEnv()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 5. Validation Check
	if cfg.Database.DSN == "" {
		return nil, errors.New("database connection DSN (POSTGRES_DSN) is required")
	}
	if cfg.Auth.JWTSecret == "" {
		return nil, errors.New("JWT secret (JWT_SECRET) is required")
	}
	if cfg.RateLimit.RequestsPerMinute <= 0 {
		return nil, errors.New("rate_limit.requests_per_minute must be > 0")
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
