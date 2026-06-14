package handler

import (
	"errors"
	"net/http"
	"net/url"
	"path"
	"time"

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

type updateUpstreamRequest struct {
	Name       string `json:"name" binding:"required"`
	TargetURL  string `json:"target_url" binding:"required"`
	Protocol   string `json:"protocol" binding:"required"`
	HealthPath string `json:"health_path"`
	Enabled    bool   `json:"enabled"`
}

func (h *UpstreamHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req updateUpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !validProtocols[req.Protocol] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "protocol must be 'http' or 'grpc'"})
		return
	}

	u := &model.Upstream{
		Name:       req.Name,
		TargetURL:  req.TargetURL,
		Protocol:   req.Protocol,
		HealthPath: req.HealthPath,
		Enabled:    req.Enabled,
	}

	if err := h.repo.Update(c.Request.Context(), id, u); err != nil {
		if errors.Is(err, storage.ErrUpstreamNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "upstream not found"})
			return
		}
		h.logger.Error().Err(err).Str("upstream_id", id).Msg("failed to update upstream")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"upstream": u})
}

func (h *UpstreamHandler) Disable(c *gin.Context) {
	id := c.Param("id")

	if err := h.repo.Disable(c.Request.Context(), id); err != nil {
		if errors.Is(err, storage.ErrUpstreamNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "upstream not found"})
			return
		}
		h.logger.Error().Err(err).Str("upstream_id", id).Msg("failed to disable upstream")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "upstream disabled", "id": id})
}

func (h *UpstreamHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, storage.ErrUpstreamNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "upstream not found"})
			return
		}
		h.logger.Error().Err(err).Str("upstream_id", id).Msg("failed to delete upstream")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "upstream deleted", "id": id})
}

func (h *UpstreamHandler) HealthCheck(c *gin.Context) {
	id := c.Param("id")

	u, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrUpstreamNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "upstream not found"})
			return
		}
		h.logger.Error().Err(err).Str("upstream_id", id).Msg("failed to load upstream for health check")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if u.Protocol != "http" {
		c.JSON(http.StatusOK, gin.H{
			"status": "unsupported",
			"detail": "health checks are only supported for http upstreams",
		})
		return
	}

	healthURL := u.TargetURL
	if u.HealthPath != "" {
		base, err := url.Parse(u.TargetURL)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"status": "unhealthy",
				"error":  "invalid target_url",
			})
			return
		}
		base.Path = path.Join(base.Path, u.HealthPath)
		healthURL = base.String()
	}

	client := http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, healthURL, nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status": "unhealthy",
			"error":  "invalid health check URL",
		})
		return
	}

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	status := "unhealthy"
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		status = "healthy"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":        status,
		"status_code":   resp.StatusCode,
		"response_time": duration.String(),
	})
}
