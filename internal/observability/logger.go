package observability

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"

	"edgecore/internal/config"
)

// NewLogger builds and returns a zerolog.Logger.
// Caller stores it on the App struct — no global state.
func NewLogger(cfg config.LogConfig) zerolog.Logger {

    // ── 1. Lumberjack: rotating file writer ──────────────────────────
    fileWriter := &lumberjack.Logger{
        Filename:   cfg.File,       // "logs/gateway.log"
        MaxSize:    cfg.MaxSizeMB,  // MB before rotation
        MaxBackups: cfg.MaxBackups, // old files to keep
        MaxAge:     cfg.MaxAgeDays, // days before deletion
        Compress:   true,           // gzip old files → gateway.log.1.gz
    }

    // ── 2. Build output writers ───────────────────────────────────────
    var writers []io.Writer

    // Always write JSON to file
    writers = append(writers, fileWriter)

    // In debug mode: pretty console output for local dev
    // In production: JSON to stdout (works with Grafana Loki, ELK etc.)
    if cfg.Level == "debug" {
        writers = append(writers, zerolog.ConsoleWriter{
            Out:        os.Stdout,
            TimeFormat: time.RFC3339,
        })
    } else {
        writers = append(writers, os.Stdout)
    }

    // ── 3. Fan-out to all writers ─────────────────────────────────────
    multi := zerolog.MultiLevelWriter(writers...)

    // ── 4. Set global log level ───────────────────────────────────────
    zerolog.SetGlobalLevel(parseLevel(cfg.Level))

    // ── 5. Build and return the logger ───────────────────────────────
    return zerolog.New(multi).
        With().
        Timestamp().
        Str("service", "edgecore-gateway").
        Logger()
}

func parseLevel(l string) zerolog.Level {
    switch l {
    case "debug":
        return zerolog.DebugLevel
    case "warn":
        return zerolog.WarnLevel
    case "error":
        return zerolog.ErrorLevel
    default:
        return zerolog.InfoLevel
    }
}