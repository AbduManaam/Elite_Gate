package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"elitegate/internal/config"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient builds a Redis client with robust connection pooling.
// Supports standard host:port addresses (e.g., "localhost:6379"), redis:// URLs,
// and rediss:// TLS URLs for production (e.g., AWS ElastiCache with Encryption in Transit).
func NewRedisClient(addr, password string, db int) (*redis.Client, error) {
	var opts *redis.Options

	if strings.HasPrefix(addr, "redis://") || strings.HasPrefix(addr, "rediss://") {
		parsedOpts, err := redis.ParseURL(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid redis url %q: %w", addr, err)
		}
		opts = parsedOpts
		if password != "" && opts.Password == "" {
			opts.Password = password
		}
		if db != 0 && opts.DB == 0 {
			opts.DB = db
		}
	} else {
		opts = &redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}
	}

	if opts.DialTimeout == 0 {
		opts.DialTimeout = 3 * time.Second
	}
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 2 * time.Second
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = 2 * time.Second
	}
	if opts.PoolSize == 0 {
		opts.PoolSize = 20
	}
	if opts.MinIdleConns == 0 {
		opts.MinIdleConns = 5
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}
	return rdb, nil
}

// NewRedis builds a Redis client using the injected RedisConfig struct.
func NewRedis(cfg config.RedisConfig) (*redis.Client, error) {
	return NewRedisClient(cfg.Addr, cfg.Password, cfg.DB)
}
