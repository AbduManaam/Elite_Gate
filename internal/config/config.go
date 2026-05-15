package config


type LogConfig struct {
    Level      string `mapstructure:"level"`
    File       string `mapstructure:"file"`
    MaxSizeMB  int    `mapstructure:"max_size_mb"`
    MaxBackups int    `mapstructure:"max_backups"`
    MaxAgeDays int    `mapstructure:"max_age_days"`
}

type Config struct {
    Server  ServerConfig  `mapstructure:"server"`
    Log     LogConfig     `mapstructure:"log"`       // ← add this line
    // your other configs...
}