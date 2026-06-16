package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type ApiKeyHandler struct {
	repo   *storage.ApiKeyRepo
	logger zerolog.Logger
}

func NewApiKeyHandler(repo *storage.ApiKeyRepo, logger zerolog.Logger) *ApiKeyHandler {
	return &ApiKeyHandler{
		repo:   repo,
		logger: logger.With().Str("handler", "api_key").Logger(),
	}
}

type createApiKeyRequest struct {
	Name      string     `json:"name" binding:"required"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (h *ApiKeyHandler) Create(c *gin.Context) {
	var req createApiKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rawKey, err := generateRawKey()
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to generate random api key")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().Str("name", req.Name).Msg("creating API key in database")

	record, err := h.repo.Create(c.Request.Context(), req.Name, rawKey, req.ExpiresAt)
	if err != nil {
		h.logger.Error().Err(err).Str("name", req.Name).Msg("failed to save API key in database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info().Str("key_id", record.ID).Str("name", record.Name).Msg("API key created successfully")

	// Return root fields as well as both nested options to be highly compatible with Postman tests
	c.JSON(http.StatusCreated, gin.H{
		"id":         record.ID,
		"project_id": record.ProjectID,
		"name":       record.Name,
		"status":     record.Status,
		"expires_at": record.ExpiresAt,
		"created_at": record.CreatedAt,
		"updated_at": record.UpdatedAt,
		"api_key":    rawKey,
		"raw_key":    rawKey,
	})
}

func (h *ApiKeyHandler) Rotate(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key id is required"})
		return
	}

	rawKey, err := generateRawKey()
	if err != nil {
		h.logger.Error().Err(err).Str("key_id", id).Msg("failed to generate new random api key for rotation")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().Str("old_key_id", id).Msg("rotating API key in database")

	record, err := h.repo.Rotate(c.Request.Context(), id, rawKey)
	if err != nil {
		if errors.Is(err, storage.ErrAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
			return
		}
		h.logger.Error().Err(err).Str("old_key_id", id).Msg("failed to rotate API key in database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().Str("old_key_id", id).Str("new_key_id", record.ID).Msg("API key rotated successfully")

	c.JSON(http.StatusOK, gin.H{
		"id":         record.ID,
		"project_id": record.ProjectID,
		"name":       record.Name,
		"status":     record.Status,
		"expires_at": record.ExpiresAt,
		"created_at": record.CreatedAt,
		"updated_at": record.UpdatedAt,
		"api_key":    rawKey,
		"raw_key":    rawKey,
	})
}

func (h *ApiKeyHandler) Revoke(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key id is required"})
		return
	}

	h.logger.Info().Str("key_id", id).Msg("revoking API key in database")

	err := h.repo.Revoke(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
			return
		}
		h.logger.Error().Err(err).Str("key_id", id).Msg("failed to revoke API key in database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().Str("key_id", id).Msg("API key revoked successfully")
	c.JSON(http.StatusOK, gin.H{
		"message": "api key revoked",
		"id":      id,
	})
}

func (h *ApiKeyHandler) List(c *gin.Context) {
	h.logger.Info().Msg("listing all API keys for project")

	keys, err := h.repo.ListAll(c.Request.Context())
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to list API keys from database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"api_keys": keys,
	})
}

func generateRawKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
