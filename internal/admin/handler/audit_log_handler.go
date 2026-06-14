package handler

import (
	"net/http"
	"strconv"

	"elitegate/internal/model"
	"elitegate/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type AuditLogHandler struct {
	repo   *storage.AuditLogRepo
	logger zerolog.Logger
}

func NewAuditLogHandler(repo *storage.AuditLogRepo, logger zerolog.Logger) *AuditLogHandler {
	return &AuditLogHandler{
		repo:   repo,
		logger: logger.With().Str("handler", "audit_log").Logger(),
	}
}

// List handles GET /audit-logs?limit=100&offset=0
func (h *AuditLogHandler) List(c *gin.Context) {
	filter := model.AuditLogFilter{
		Limit:  parseQueryInt(c, "limit", 100),
		Offset: parseQueryInt(c, "offset", 0),
	}

	h.logger.Info().
		Int("limit", filter.Limit).
		Int("offset", filter.Offset).
		Msg("listing audit logs")

	logs, err := h.repo.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error().
			Err(err).
			Int("limit", filter.Limit).
			Int("offset", filter.Offset).
			Msg("failed to list audit logs")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve audit logs"})
		return
	}

	h.logger.Info().Int("count", len(logs)).Msg("audit logs listed successfully")
	c.JSON(http.StatusOK, gin.H{
		"audit_logs": logs,
		"count":      len(logs),
		"offset":     filter.Offset,
		"limit":      filter.Limit,
	})
}

// parseQueryInt reads an integer query param, returning fallback on missing/invalid input.
func parseQueryInt(c *gin.Context, key string, fallback int) int {
	raw := c.Query(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}
