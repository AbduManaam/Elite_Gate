package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "apikey:"
const cacheTTL = 10 * time.Minute

type APIKeyRecord struct {
	ClientID  string
	RevokedAt *time.Time
	Roles     []string
	Scopes    []string
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
func (s *RedisKeyStore) Validate(key string) (*APIKeyRecord, bool) {
	ctx := context.Background()
	keyHash := hashKey(key)
	cacheKey := keyPrefix + keyHash

	//Checking if API key is available in Redis cache
	if s.redis != nil {
		data, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var rec APIKeyRecord
			if json.Unmarshal([]byte(data), &rec) == nil {
				return &rec, true
			}
		}
	}

	if s.db == nil {
		return nil, false
	}

	// Fallback to here, when Redis cache is not available, to check api key availability in postgresql database
	record, err := s.db.FindByHash(ctx, keyHash)
	if err != nil || record == nil {
		return nil, false
	}

	if record.RevokedAt != nil {
		return nil, false
	}

	if s.redis != nil {
		if data, err := json.Marshal(record); err == nil {
			s.redis.Set(ctx, cacheKey, data, cacheTTL)
		}
	}
	return record, true
}

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)
}
