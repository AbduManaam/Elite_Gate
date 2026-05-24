package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "apikey:"
const cacheTTL = 10 * time.Minute

type APIKeyRecord struct {
	ClientID  string
	RevokedAt *time.Time
}

type KeyRepository interface {
	FindByHash(ctx context.Context, keyHash string) (*APIKeyRecord, error)
}

type RedisKeyStore struct {
	redis *redis.Client
	db    KeyRepository
}

func NewRedisKeyStore(rdb *redis.Client, db KeyRepository) *RedisKeyStore {
	return &RedisKeyStore{redis: rdb, db: db}
}

// Validate checks Redis first, falls back to PostgreSQL on miss.
func (s *RedisKeyStore) Validate(key string) (string, bool) {
	ctx := context.Background()
	keyHash := hashKey(key)
	cacheKey := keyPrefix + keyHash

	clientID, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		return clientID, true
	}

	if s.db == nil {
		return "", false
	}

	record, err := s.db.FindByHash(ctx, keyHash)
	if err != nil || record == nil {
		return "", false
	}

	if record.RevokedAt != nil {
		return "", false
	}

	s.redis.Set(ctx, cacheKey, record.ClientID, cacheTTL)
	return record.ClientID, true
}

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)
}
