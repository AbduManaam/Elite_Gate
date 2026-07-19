package handler

import (
	"errors"
	"net/http"

	"elitegate/helper"
	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type RouteHandler struct {
	svc      *service.RouteService
	auditSvc *service.AuditService
	logger   zerolog.Logger
}

func NewRouteHandler(svc *service.RouteService, logger zerolog.Logger, auditSvc *service.AuditService) *RouteHandler {
	return &RouteHandler{
		svc:      svc,
		auditSvc: auditSvc,
		logger:   logger,
	}
}

func (h *RouteHandler) List(c *gin.Context) {
	page, limit, offset, err := service.ParsePaginationOffset(c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	routes, total, err := h.svc.ListRoutes(c.Request.Context(), limit, offset)
	if err != nil {
		helper.RespondInternalError(c, h.logger, err, "failed to load routes")
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse[model.Route]{
		Items:      routes,
		Pagination: service.BuildPagination(page, limit, total),
	})
}

type createRouteRequest struct {
	Name       string   `json:"name"        binding:"required"`
	Path       string   `json:"path"        binding:"required"`
	UpstreamID string   `json:"upstream_id" binding:"required"`
	PolicyID   *string  `json:"policy_id"`
	Methods    []string `json:"methods"     binding:"required"`
	MatchType  string   `json:"match_type"`
	Enabled    bool     `json:"enabled"`
}

func (h *RouteHandler) Create(c *gin.Context) {
	var req createRouteRequest

	h.logger.Info().
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Msg("create route request received")

	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error().Err(err).Msg("failed to parse request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rt, err := h.svc.CreateRoute(c.Request.Context(), req.Name, req.Path, req.UpstreamID, req.PolicyID, req.Methods, req.MatchType, req.Enabled)
	if err != nil {
		if errors.Is(err, service.ErrInvalidMatchType) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, storage.ErrRouteNameConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "route name already exists"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("path", req.Path).Logger(), err, "internal server error")
		return
	}

	h.logger.Info().Str("route_id", rt.ID).Str("path", rt.Path).Msg("route created successfully")
	h.auditSvc.Record(c, "route.create", "route", rt.ID, rt.Path, gin.H{"name": rt.Name, "path": rt.Path, "upstream_id": rt.UpstreamID, "methods": rt.Methods, "match_type": rt.MatchType, "enabled": rt.Enabled})
	c.JSON(http.StatusCreated, gin.H{"route": rt})
}

func (h *RouteHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "route id is required"})
		return
	}

	h.logger.Info().Str("route_id", id).Msg("deleting route")

	err := h.svc.DeleteRoute(c.Request.Context(), id)
	if err == nil {
		h.logger.Info().Str("route_id", id).Msg("route deleted")
		h.auditSvc.Record(c, "route.delete", "route", id, "", nil)
		c.JSON(http.StatusOK, gin.H{"message": "route deleted", "id": id})
		return
	}

	if errors.Is(err, storage.ErrRouteNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	helper.RespondInternalError(c, h.logger.With().Str("route_id", id).Logger(), err, "internal server error")
}

func (h *RouteHandler) Update(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "route id is required"})
		return
	}

	var req createRouteRequest
	h.logger.Info().Str("route_id", id).Msg("update route request received")

	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error().Err(err).Msg("failed to parse update request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rt, err := h.svc.UpdateRoute(c.Request.Context(), id, req.Name, req.Path, req.UpstreamID, req.PolicyID, req.Methods, req.MatchType, req.Enabled)
	if err != nil {
		if errors.Is(err, service.ErrInvalidMatchType) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, storage.ErrRouteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("route_id", id).Logger(), err, "internal server error")
		return
	}

	h.logger.Info().Str("route_id", rt.ID).Str("path", rt.Path).Msg("route updated successfully")
	h.auditSvc.Record(c, "route.update", "route", id, rt.Path, gin.H{"name": rt.Name, "path": rt.Path, "upstream_id": rt.UpstreamID, "methods": rt.Methods, "match_type": rt.MatchType, "enabled": rt.Enabled})
	c.JSON(http.StatusOK, gin.H{"route": rt})
}

func (h *RouteHandler) Disable(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "route id is required"})
		return
	}

	h.logger.Info().Str("route_id", id).Msg("disabling route")

	err := h.svc.DisableRoute(c.Request.Context(), id)
	if err == nil {
		h.logger.Info().Str("route_id", id).Msg("route disabled successfully")
		h.auditSvc.Record(c, "route.update", "route", id, "", gin.H{"enabled": false})
		c.JSON(http.StatusOK, gin.H{"message": "route disabled", "id": id})
		return
	}

	if errors.Is(err, storage.ErrRouteNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	helper.RespondInternalError(c, h.logger.With().Str("route_id", id).Logger(), err, "internal server error")
}

func (h *RouteHandler) Enable(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "route id is required"})
		return
	}

	h.logger.Info().Str("route_id", id).Msg("enabling route")

	err := h.svc.EnableRoute(c.Request.Context(), id)
	if err == nil {
		h.logger.Info().Str("route_id", id).Msg("route enabled successfully")
		h.auditSvc.Record(c, "route.update", "route", id, "", gin.H{"enabled": true})
		c.JSON(http.StatusOK, gin.H{"message": "route enabled", "id": id})
		return
	}

	if errors.Is(err, storage.ErrRouteNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	helper.RespondInternalError(c, h.logger.With().Str("route_id", id).Logger(), err, "internal server error")
}
