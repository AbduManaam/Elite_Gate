package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"elitegate/helper"

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

type KeySnapshotDTO struct {
	KeyHash string   `json:"key_hash"`
	Roles   []string `json:"roles"`
	Scopes  []string `json:"scopes"`
}

type RedisKeyStore struct {
	redis *redis.Client
	db    KeyRepository
	mu    sync.RWMutex
	local map[string]*APIKeyRecord
}

func NewRedisKeyStore(rdb *redis.Client, db KeyRepository) *RedisKeyStore {
	return &RedisKeyStore{
		redis: rdb,
		db:    db,
		local: make(map[string]*APIKeyRecord),
	}
}

// UpdateLocalKeys warms the in-memory map directly from snapshot sync DTOs.
func (s *RedisKeyStore) UpdateLocalKeys(clientID string, keys []KeySnapshotDTO) {
	s.mu.Lock()
	defer s.mu.Unlock()
	newLocal := make(map[string]*APIKeyRecord, len(keys))
	for _, k := range keys {
		newLocal[k.KeyHash] = &APIKeyRecord{
			ClientID: clientID,
			Roles:    k.Roles,
			Scopes:   k.Scopes,
		}
	}
	s.local = newLocal
}

// Validate checks local memory first, then Redis, falling back to PostgreSQL on miss.
func (s *RedisKeyStore) Validate(key string) (*APIKeyRecord, bool) {
	return s.ValidateWithContext(context.Background(), key)
}

// ValidateWithContext checks local memory first, then Redis, falling back to PostgreSQL on miss.
func (s *RedisKeyStore) ValidateWithContext(ctx context.Context, key string) (*APIKeyRecord, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false
	}
	keyHash := hashKey(key)

	// 1. Check local in-memory snapshot cache first
	s.mu.RLock()
	rec, ok := s.local[keyHash]
	s.mu.RUnlock()
	if ok && rec != nil {
		if rec.RevokedAt != nil && rec.RevokedAt.Before(time.Now()) {
			return nil, false
		}
		return rec, true
	}

	// 2. Checking if API key is available in Redis cache
	cacheKey := helper.PrefixedKey(keyPrefix + keyHash)
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

	// 3. Fallback to check api key availability in postgresql database
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
	key = strings.TrimSpace(key)
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)
}
