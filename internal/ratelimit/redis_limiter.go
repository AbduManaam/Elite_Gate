package ratelimit

//sliding window counter
import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

type RedisLimiter struct {
    rdb *redis.Client
}

func NewRedisLimiter(rdb *redis.Client) *RedisLimiter {
    return &RedisLimiter{rdb: rdb}
}

func (l *RedisLimiter) Allow(ctx context.Context, clientID, route string, rpm int) (bool, error) {
    now := time.Now()
    window := now.Unix() / 60       // 1-minute bucket
    key := fmt.Sprintf("rate:%s:%s:%d", clientID, route, window)

    count, err := l.rdb.Incr(ctx, key).Result()
    if err != nil {
        return false, err
    }
    if count == 1 {
        l.rdb.Expire(ctx, key, 2*time.Minute)
    }
    return int(count) <= rpm, nil
}