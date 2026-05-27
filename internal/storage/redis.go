package storage

import (
	"context"
	"fmt"
	"time"

	"elitegate/internal/config"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient builds a Redis client with robust connection pooling.
func NewRedisClient(addr, password string, db int) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolSize:     20, // max 20 active connections
		MinIdleConns: 5,  // keeps 5 ready-to-use idle connections
	})

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
