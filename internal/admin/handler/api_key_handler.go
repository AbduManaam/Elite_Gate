package handler

import (
	"errors"
	"net/http"
	"time"

	"elitegate/helper"
	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type ApiKeyHandler struct {
	svc      *service.ApiKeyService
	auditSvc *service.AuditService
	logger   zerolog.Logger
}

func NewApiKeyHandler(svc *service.ApiKeyService, logger zerolog.Logger, auditSvc *service.AuditService) *ApiKeyHandler {
	return &ApiKeyHandler{
		svc:      svc,
		auditSvc: auditSvc,
		logger:   logger.With().Str("handler", "api_key").Logger(),
	}
}

type createApiKeyRequest struct {
	Name      string     `json:"name" binding:"required"`
	ExpiresAt *time.Time `json:"expires_at"`
	Roles     []string   `json:"roles"`
	Scopes    []string   `json:"scopes"`
}

func (h *ApiKeyHandler) Create(c *gin.Context) {
	var req createApiKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info().Str("name", req.Name).Msg("creating API key in database")

	record, rawKey, err := h.svc.CreateApiKey(c.Request.Context(), req.Name, req.ExpiresAt, req.Roles, req.Scopes)
	if err != nil {
		helper.RespondInternalError(c, h.logger.With().Str("name", req.Name).Logger(), err, "failed to save API key in database")
		return
	}

	h.logger.Info().Str("api_key_id", record.ID).Str("name", record.Name).Msg("API key created successfully")

	h.auditSvc.Record(c, "api_key.create", "api_key", record.ID, record.Name, gin.H{
		"name":       record.Name,
		"expires_at": record.ExpiresAt,
		"roles":      record.Roles,
		"scopes":     record.Scopes,
	})

	c.JSON(http.StatusCreated, gin.H{
		"api_key": record,
		"raw_key": rawKey,
	})
}

func (h *ApiKeyHandler) Rotate(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "api key id is required"})
		return
	}

	h.logger.Info().Str("api_key_id", id).Msg("rotating API key")

	newRawKey, err := h.svc.RotateApiKey(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
			return
		}
		if errors.Is(err, storage.ErrAPIKeyRevoked) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot rotate a revoked api key"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("api_key_id", id).Logger(), err, "failed to rotate API key")
		return
	}

	h.logger.Info().Str("api_key_id", id).Msg("API key rotated successfully")
	h.auditSvc.Record(c, "api_key.rotate", "api_key", id, "", nil)

	c.JSON(http.StatusOK, gin.H{
		"id":      id,
		"raw_key": newRawKey,
	})
}

func (h *ApiKeyHandler) Revoke(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "api key id is required"})
		return
	}

	h.logger.Info().Str("api_key_id", id).Msg("revoking API key")

	err := h.svc.RevokeApiKey(c.Request.Context(), id)
	if err == nil {
		h.logger.Info().Str("api_key_id", id).Msg("API key revoked successfully")
		h.auditSvc.Record(c, "api_key.revoke", "api_key", id, "", nil)
		c.JSON(http.StatusOK, gin.H{"message": "api key revoked", "id": id})
		return
	}

	if errors.Is(err, storage.ErrAPIKeyNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
		return
	}

	helper.RespondInternalError(c, h.logger.With().Str("api_key_id", id).Logger(), err, "failed to revoke API key")
}

func (h *ApiKeyHandler) List(c *gin.Context) {
	page, limit, offset, err := service.ParsePaginationOffset(c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info().Int("page", page).Int("limit", limit).Msg("listing API keys")

	keys, total, err := h.svc.ListApiKeys(c.Request.Context(), limit, offset)
	if err != nil {
		helper.RespondInternalError(c, h.logger, err, "failed to list API keys")
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse[storage.ApiKeyRecord]{
		Items:      keys,
		Pagination: service.BuildPagination(page, limit, total),
	})
}
