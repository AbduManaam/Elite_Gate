package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var luaScript = redis.NewScript(`
 local key = KEYS[1]
 local limit = tonumber(ARGV[1])
 local window = tonumber(ARGV[2])
 local current = redis.call("INCR", key)
 if current == 1 then
 redis.call("EXPIRE", key, window)
 end
 return current
`)

type RedisLimiter struct {
	client         *redis.Client
	requestsPerMin int
	fallback       Limiter
}

func NewRedisLimiter(client *redis.Client, rpm int, fallback Limiter) *RedisLimiter {
	return &RedisLimiter{
		client:         client,
		requestsPerMin: rpm,
		fallback:       fallback,
	}
}

func (r *RedisLimiter) AllowWithLimit(key string, limit int) bool {
	if r.client == nil {
		return r.fallback.AllowWithLimit(key, limit)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	windowKey := fmt.Sprintf("ratelimit:%s:%d", key, time.Now().Unix()/60)
	result, err := luaScript.Run(ctx, r.client,
		[]string{windowKey},
		limit,
		60,
	).Int()
	if err != nil {
		return r.fallback.AllowWithLimit(key, limit)
	}
	return result <= limit
}

func (r *RedisLimiter) Allow(key string) bool {
	return r.AllowWithLimit(key, r.requestsPerMin)
}

func (r *RedisLimiter) Count(key string) int {
	if r.client == nil {
		return r.fallback.Count(key)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	windowKey := fmt.Sprintf("ratelimit:%s:%d", key, time.Now().Unix()/60)
	val, err := r.client.Get(ctx, windowKey).Int()
	if err != nil {
		return r.fallback.Count(key)
	}
	return val
}

func (r *RedisLimiter) Limit() int {
	return r.requestsPerMin
}