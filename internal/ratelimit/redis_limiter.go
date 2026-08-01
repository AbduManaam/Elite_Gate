package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// slidingWindowScript atomically, in one round trip:
//  1. Trims entries older than the window from the ZSET.
//  2. Reads the current count (ZCARD).
//  3. If under the limit, records this request and returns allowed=1.
//  4. If at/over the limit, does NOT record the request and returns allowed=0.
//
// Capping the ZSET to maxEntries prevents unbounded memory growth for a
// single key under pathological burst traffic, independent of the window
// trim (e.g. a client hammering the endpoint faster than the window can
// naturally shed old entries).
var slidingWindowScript = redis.NewScript(`
local key         = KEYS[1]
local now_ms      = tonumber(ARGV[1])
local window_ms   = tonumber(ARGV[2])
local limit       = tonumber(ARGV[3])
local member      = ARGV[4]
local max_entries = tonumber(ARGV[5])

redis.call("ZREMRANGEBYSCORE", key, "-inf", now_ms - window_ms)

local current = redis.call("ZCARD", key)
if current >= limit then
    return {0, current}
end

redis.call("ZADD", key, now_ms, member)
redis.call("ZREMRANGEBYRANK", key, 0, -max_entries - 1) -- hard cap safety valve
redis.call("PEXPIRE", key, window_ms)

return {1, current + 1}
`)

var countScript = redis.NewScript(`
local key       = KEYS[1]
local now_ms    = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
redis.call("ZREMRANGEBYSCORE", key, "-inf", now_ms - window_ms)
return redis.call("ZCARD", key)
`)

const (
	slidingWindow    = 60 * time.Second
	maxEntriesPerKey = 10000 // safety valve — see script comment
)

type RedisLimiter struct {
	client         *redis.Client
	requestsPerMin int
	fallback       Limiter
	now            func() time.Time
}

func NewRedisLimiter(client *redis.Client, rpm int, fallback Limiter) *RedisLimiter {
	return &RedisLimiter{
		client:         client,
		requestsPerMin: rpm,
		fallback:       fallback,
		now:            time.Now,
	}
}

func (r *RedisLimiter) CheckAndConsume(key string, limit int) RateResult {
	now := r.now()
	resetAt := now.Add(slidingWindow)

	if r.client == nil {
		return r.fallback.CheckAndConsume(key, limit)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	windowKey := fmt.Sprintf("ratelimit:%s", key)
	nowMs := now.UnixMilli()

	res, err := slidingWindowScript.Run(ctx, r.client,
		[]string{windowKey},
		nowMs, slidingWindow.Milliseconds(), limit, uuid.NewString(), maxEntriesPerKey,
	).Result()
	if err != nil {
		// Fail open to the in-memory fallback rather than failing the request —
		// preserves current behavior; a Redis blip should not itself become
		// an outage for every downstream API call.
		return r.fallback.CheckAndConsume(key, limit)
	}

	vals := res.([]interface{})
	allowed := vals[0].(int64) == 1
	count := int(vals[1].(int64))
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	return RateResult{Allowed: allowed, Remaining: remaining, ResetAt: resetAt}
}

// AllowWithLimit / Allow / Count kept for interface compatibility with any
// existing callers outside the gateway middleware (e.g. tests).
func (r *RedisLimiter) AllowWithLimit(key string, limit int) bool {
	return r.CheckAndConsume(key, limit).Allowed
}
func (r *RedisLimiter) Allow(key string) bool { return r.AllowWithLimit(key, r.requestsPerMin) }

func (r *RedisLimiter) Count(key string) int {
	if r.client == nil {
		return r.fallback.Count(key)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	windowKey := fmt.Sprintf("ratelimit:%s", key)
	val, err := countScript.Run(ctx, r.client, []string{windowKey},
		r.now().UnixMilli(), slidingWindow.Milliseconds()).Int()
	if err != nil {
		return r.fallback.Count(key)
	}
	return val
}

func (r *RedisLimiter) Limit() int { return r.requestsPerMin }
