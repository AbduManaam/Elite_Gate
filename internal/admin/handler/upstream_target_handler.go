package handler

import (
	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type UpstreamTargetHandler struct {
	repo     *storage.UpstreamTargetRepo
	auditSvc *service.AuditService
	logger   zerolog.Logger
}

func NewUpstreamTargetHandler(repo *storage.UpstreamTargetRepo, logger zerolog.Logger, auditSvc *service.AuditService) *UpstreamTargetHandler {
	return &UpstreamTargetHandler{
		repo:     repo,
		auditSvc: auditSvc,
		logger:   logger,
	}
}

type addTargetRequest struct {
	TargetURL string `json:"target_url" validate:"required"`
	Weight    int    `json:"weight"`
	Enabled   *bool  `json:"enabled"`
}

func (h *UpstreamTargetHandler) Add(c *gin.Context) {
	upstreamID := c.Param("id")
	if upstreamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upstream id is required"})
		return
	}

	var req addTargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	weight := req.Weight
	if weight <= 0 {
		weight = 1
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	t := &model.UpstreamTarget{
		UpstreamID: upstreamID,
		TargetURL:  req.TargetURL,
		Weight:     weight,
		Enabled:    enabled,
	}

	if err := h.repo.Add(c.Request.Context(), t); err != nil {
		h.logger.Error().Err(err).
			Str("upstream_id", upstreamID).
			Str("target_url", req.TargetURL).
			Msg("failed to add upstream target")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().
		Str("upstream_id", upstreamID).
		Str("target_id", t.ID).
		Str("target_url", t.TargetURL).
		Msg("upstream target added")

	h.auditSvc.Record(c, "upstream.update", "upstream", upstreamID, t.TargetURL, gin.H{"action": "target_add", "target_url": t.TargetURL, "weight": t.Weight, "enabled": t.Enabled})

	c.JSON(http.StatusCreated, gin.H{"target": t})
}

func (h *UpstreamTargetHandler) List(c *gin.Context) {
	UpstreamID := c.Param("id")
	if UpstreamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"Error": "upstream id is required"})
		return
	}
	target, err := h.repo.ListByUpstream(c.Request.Context(), UpstreamID)
	if err != nil {
		h.logger.Error().Err(err).
			Str("Upstream_id", UpstreamID).
			Msg("failed to list upstream targets")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"targets": target})

}

func (h *UpstreamTargetHandler) Remove(c *gin.Context) {
	targetID := c.Param("targetId")
	if targetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target id is required"})
		return
	}

	if err := h.repo.Remove(c.Request.Context(), targetID); err != nil {
		if errors.Is(err, storage.ErrUpstreamTargetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "target not found"})
			return
		}
		h.logger.Error().Err(err).
			Str("target_id", targetID).
			Msg("failed to remove upstream target")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().Str("target_id", targetID).Msg("upstream target removed")
	h.auditSvc.Record(c, "upstream.update", "upstream", targetID, "", gin.H{"action": "target_remove", "target_id": targetID})
	c.JSON(http.StatusOK, gin.H{"message": "target removed", "id": targetID})
}
