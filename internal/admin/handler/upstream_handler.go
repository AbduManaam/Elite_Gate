package handler

import (
	"errors"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"elitegate/helper"
	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"
)

type UpstreamHandler struct {
	svc      *service.UpstreamService
	auditSvc *service.AuditService
	logger   zerolog.Logger
}

func NewUpstreamHandler(
	svc *service.UpstreamService,
	logger zerolog.Logger,
	auditSvc *service.AuditService,
) *UpstreamHandler {
	return &UpstreamHandler{
		svc:      svc,
		auditSvc: auditSvc,
		logger:   logger,
	}
}

func (h *UpstreamHandler) List(c *gin.Context) {
	page, limit, offset, err := service.ParsePaginationOffset(c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info().Int("page", page).Int("limit", limit).Msg("listing upstreams")

	upstreams, total, err := h.svc.ListUpstreams(c.Request.Context(), limit, offset)
	if err != nil {
		helper.RespondInternalError(c, h.logger, err, "failed to load upstreams")
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse[model.Upstream]{
		Items:      upstreams,
		Pagination: service.BuildPagination(page, limit, total),
	})
}

type createUpstreamRequest struct {
	Name       string `json:"name" binding:"required"`
	TargetURL  string `json:"target_url" binding:"required"`
	Protocol   string `json:"protocol" binding:"required"`
	HealthPath string `json:"health_path"`
	Enabled    bool   `json:"enabled"`
	LBStrategy string `json:"lb_strategy"`
}

func (h *UpstreamHandler) Create(c *gin.Context) {
	var req createUpstreamRequest

	h.logger.Info().
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Msg("create upstream request received")

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	u, err := h.svc.CreateUpstream(c.Request.Context(), req.Name, req.TargetURL, req.Protocol, req.HealthPath, req.LBStrategy, req.Enabled)
	if err != nil {
		if errors.Is(err, service.ErrInvalidProtocol) || errors.Is(err, service.ErrInvalidLBStrategy) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, storage.ErrUpstreamNameConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "upstream name already exists"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("name", req.Name).Logger(), err, "internal server error")
		return
	}

	h.logger.Info().
		Str("upstream_id", u.ID).
		Str("name", u.Name).
		Msg("upstream created successfully")

	h.auditSvc.Record(c, "upstream.create", "upstream", u.ID, u.Name, gin.H{"name": u.Name, "target_url": u.TargetURL, "protocol": u.Protocol, "health_path": u.HealthPath, "enabled": u.Enabled, "lb_strategy": u.LBStrategy})

	c.JSON(http.StatusCreated, gin.H{"upstream": u})
}

type updateUpstreamRequest struct {
	Name       string `json:"name" binding:"required"`
	TargetURL  string `json:"target_url" binding:"required"`
	Protocol   string `json:"protocol" binding:"required"`
	HealthPath string `json:"health_path"`
	Enabled    bool   `json:"enabled"`
	LBStrategy string `json:"lb_strategy"`
}

func (h *UpstreamHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req updateUpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	u, err := h.svc.UpdateUpstream(c.Request.Context(), id, req.Name, req.TargetURL, req.Protocol, req.HealthPath, req.LBStrategy, req.Enabled)
	if err != nil {
		if errors.Is(err, service.ErrInvalidProtocol) || errors.Is(err, service.ErrInvalidLBStrategy) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, storage.ErrUpstreamNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "upstream not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("upstream_id", id).Logger(), err, "internal server error")
		return
	}

	h.auditSvc.Record(c, "upstream.update", "upstream", id, u.Name, gin.H{"name": u.Name, "target_url": u.TargetURL, "protocol": u.Protocol, "health_path": u.HealthPath, "enabled": u.Enabled, "lb_strategy": u.LBStrategy})
	c.JSON(http.StatusOK, gin.H{"upstream": u})
}

func (h *UpstreamHandler) Disable(c *gin.Context) {
	id := c.Param("id")

	if err := h.svc.DisableUpstream(c.Request.Context(), id); err != nil {
		if errors.Is(err, storage.ErrUpstreamNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "upstream not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("upstream_id", id).Logger(), err, "internal server error")
		return
	}

	h.auditSvc.Record(c, "upstream.update", "upstream", id, "", gin.H{"enabled": false})
	c.JSON(http.StatusOK, gin.H{"message": "upstream disabled", "id": id})
}

func (h *UpstreamHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.svc.DeleteUpstream(c.Request.Context(), id); err != nil {
		if errors.Is(err, storage.ErrUpstreamNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "upstream not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("upstream_id", id).Logger(), err, "internal server error")
		return
	}

	h.auditSvc.Record(c, "upstream.delete", "upstream", id, "", nil)
	c.JSON(http.StatusOK, gin.H{"message": "upstream deleted", "id": id})
}

func (h *UpstreamHandler) HealthCheck(c *gin.Context) {
	id := c.Param("id")

	u, err := h.svc.GetUpstreamByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrUpstreamNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "upstream not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("upstream_id", id).Logger(), err, "internal server error")
		return
	}

	if u.Protocol != "http" {
		c.JSON(http.StatusOK, gin.H{
			"status": "unsupported",
			"reason": "health check probe is only supported for http protocol",
		})
		return
	}

	healthPath := u.HealthPath
	if healthPath == "" {
		healthPath = "/health"
	}

	parsed, err := url.Parse(u.TargetURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status": "unhealthy",
			"reason": "invalid target_url",
		})
		return
	}
	parsed.Path = path.Join(parsed.Path, healthPath)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(parsed.String())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status": "unhealthy",
			"reason": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status":      "unhealthy",
			"status_code": resp.StatusCode,
		})
	}
}
