package handler

import (
	"net/http"
	"strconv"
	"time"

	"elitegate/helper"
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
		Limit:  parseQueryInt(c, "limit", 20),
		Offset: parseQueryInt(c, "offset", 0),
		Actor:  c.Query("actor"),
		Action: c.Query("action"),
	}

	if from := c.Query("date_from"); from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			filter.DateFrom = &t
		}
	}
	if to := c.Query("date_to"); to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			t = t.Add(24 * time.Hour) // inclusive end-of-day
			filter.DateTo = &t
		}
	}

	h.logger.Info().
		Int("limit", filter.Limit).
		Int("offset", filter.Offset).
		Msg("listing audit logs")

	page, err := h.repo.List(c.Request.Context(), filter)
	if err != nil {
		helper.RespondInternalError(c, h.logger.With().Int("limit", filter.Limit).Int("offset", filter.Offset).Logger(), err, "failed to retrieve audit logs")
		return
	}

	h.logger.Info().Int("count", len(page.Logs)).Msg("audit logs listed successfully")
	c.JSON(http.StatusOK, gin.H{
		"audit_logs": page.Logs,
		"total":      page.Total,
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
