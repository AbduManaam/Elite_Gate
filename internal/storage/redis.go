package storage

import (
    "context"
    "fmt"
    "os"

    "github.com/redis/go-redis/v9"
)

func NewRedis() (*redis.Client, error) {
    addr := os.Getenv("REDIS_ADDR")       // redis:6379
    pass := os.Getenv("REDIS_PASSWORD")   // redis_secret

    rdb := redis.NewClient(&redis.Options{
        Addr:     addr,
        Password: pass,
        DB:       0,
    })

    if err := rdb.Ping(context.Background()).Err(); err != nil {
        return nil, fmt.Errorf("redis ping failed: %w", err)
    }
    return rdb, nil
}