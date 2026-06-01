package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"elitegate/internal/model"
	"elitegate/internal/storage"
)

type UpstreamHandler struct {
	repo   *storage.UpstreamRepo
	logger zerolog.Logger
}

func NewUpstreamHandler(
	repo *storage.UpstreamRepo,
	logger zerolog.Logger,
) *UpstreamHandler {
	return &UpstreamHandler{
		repo:   repo,
		logger: logger,
	}
}

func (h *UpstreamHandler) List(c *gin.Context) {
	h.logger.Info().
		Msg("listing upstreams")

	upstreams, err := h.repo.ListAll(c.Request.Context())
	if err != nil {
		h.logger.Error().
			Err(err).
			Msg("failed to list upstreams")

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
		return
	}

	h.logger.Info().
		Int("count", len(upstreams)).
		Msg("upstreams loaded")

	c.JSON(http.StatusOK, gin.H{
		"upstreams": upstreams,
	})
}

// validProtocols is the allowed set of backend protocols.
// Protocol is a property of the upstream (what the backend speaks), not the route.
var validProtocols = map[string]bool{"http": true, "grpc": true}

type createUpstreamRequest struct {
	Name       string `json:"name" binding:"required"`
	TargetURL  string `json:"target_url" binding:"required"`
	Protocol   string `json:"protocol" binding:"required"`
	HealthPath string `json:"health_path"`
	Enabled    bool   `json:"enabled"`
}

func (h *UpstreamHandler) Create(c *gin.Context) {
	var req createUpstreamRequest

	h.logger.Info().
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Msg("create upstream request received")

	// Parse JSON
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error().
			Err(err).
			Msg("failed to parse upstream request")

		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	h.logger.Info().
		Interface("request", req).
		Msg("upstream request parsed")

	// Validate protocol
	if !validProtocols[req.Protocol] {
		h.logger.Warn().
			Str("protocol", req.Protocol).
			Msg("invalid protocol")

		c.JSON(http.StatusBadRequest, gin.H{
			"error": "protocol must be 'http' or 'grpc'",
		})
		return
	}

	u := &model.Upstream{
		Name:       req.Name,
		TargetURL:  req.TargetURL,
		Protocol:   req.Protocol,
		HealthPath: req.HealthPath,
		Enabled:    req.Enabled,
	}

	h.logger.Info().
		Str("name", u.Name).
		Str("target_url", u.TargetURL).
		Str("protocol", u.Protocol).
		Msg("creating upstream")

	// Save to database
	if err := h.repo.Create(c.Request.Context(), u); err != nil {
		h.logger.Error().
			Err(err).
			Str("name", u.Name).
			Msg("failed to create upstream")

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
		return
	}

	h.logger.Info().
		Str("upstream_id", u.ID).
		Str("name", u.Name).
		Msg("upstream created successfully")

	c.JSON(http.StatusCreated, gin.H{
		"upstream": u,
	})
}