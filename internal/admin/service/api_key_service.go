package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

type ApiKeyService struct {
	repo   *storage.ApiKeyRepo
	logger zerolog.Logger
}

func NewApiKeyService(repo *storage.ApiKeyRepo, logger zerolog.Logger) *ApiKeyService {
	return &ApiKeyService{
		repo:   repo,
		logger: logger.With().Str("service", "api_key").Logger(),
	}
}

func GenerateRawKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return "cg_" + hex.EncodeToString(b), nil
}

func (s *ApiKeyService) CreateApiKey(ctx context.Context, name string, expiresAt *time.Time, roles, scopes []string) (*storage.ApiKeyRecord, string, error) {
	rawKey, err := GenerateRawKey()
	if err != nil {
		return nil, "", err
	}

	record, err := s.repo.Create(ctx, name, rawKey, expiresAt, roles, scopes)
	if err != nil {
		return nil, "", fmt.Errorf("create api key: %w", err)
	}

	return record, rawKey, nil
}

func (s *ApiKeyService) RotateApiKey(ctx context.Context, id string) (string, error) {
	newRawKey, err := GenerateRawKey()
	if err != nil {
		return "", err
	}

	if _, err := s.repo.Rotate(ctx, id, newRawKey); err != nil {
		return "", fmt.Errorf("rotate api key: %w", err)
	}

	return newRawKey, nil
}

func (s *ApiKeyService) RevokeApiKey(ctx context.Context, id string) error {
	if err := s.repo.Revoke(ctx, id); err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	return nil
}

func (s *ApiKeyService) ListApiKeys(ctx context.Context, limit, offset int) ([]storage.ApiKeyRecord, int, error) {
	keys, total, err := s.repo.ListAll(ctx, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list api keys: %w", err)
	}
	return keys, total, nil
}
