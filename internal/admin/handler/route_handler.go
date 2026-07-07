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

type RouteHandler struct {
	repo   *storage.RouteRepo
	logger zerolog.Logger
}

// Receive route-related HTTP requests, call the repository to perform database operations, and return HTTP responses.
func NewRouteHandler(repo *storage.RouteRepo, logger zerolog.Logger) *RouteHandler {
	return &RouteHandler{
		repo:   repo,
		logger: logger,
	}
}

func (h *RouteHandler) List(c *gin.Context) {
	page, limit, offset, err := service.ParsePaginationOffset(c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	routes, total, err := h.repo.ListAll(c.Request.Context(), limit, offset)
	if err != nil {
		h.logger.Debug().Err(err).Str("handler", "ListRoutes").Msg("failed to list routes")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load routes"})
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse[model.Route]{
		Items:      routes,
		Pagination: service.BuildPagination(page, limit, total),
	})
}

// createRouteRequest is the API contract for creating or updating a route.
// upstream_id and policy_id reference the normalized tables.
// methods replaces the old TEXT[] column — each entry becomes a row in route_methods.
type createRouteRequest struct {
	Name       string   `json:"name"        binding:"required"`
	Path       string   `json:"path"        binding:"required"`
	UpstreamID string   `json:"upstream_id" binding:"required"`
	PolicyID   *string  `json:"policy_id"`
	Methods    []string `json:"methods"     binding:"required"`
	MatchType  string   `json:"match_type"`
	Enabled    bool     `json:"enabled"`
}

var validMatchTypes = map[string]bool{"exact": true, "prefix": true}

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

	if req.MatchType == "" {
		req.MatchType = "prefix"
	}
	if !validMatchTypes[req.MatchType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "match_type must be 'exact' or 'prefix'"})
		return
	}

	var policyID *string
	var policyIDStr string
	if req.PolicyID != nil && *req.PolicyID != "" {
		policyID = req.PolicyID
		policyIDStr = *req.PolicyID
	}

	rt := &model.Route{
		Name:       req.Name,
		Path:       req.Path,
		UpstreamID: &req.UpstreamID,
		PolicyID:   policyID,
		Methods:    req.Methods,
		MatchType:  req.MatchType,
		Enabled:    req.Enabled,
	}

	h.logger.Info().
		Str("path", rt.Path).
		Str("upstream_id", req.UpstreamID).
		Str("policy_id", policyIDStr).
		Msg("creating route in database")

	if err := h.repo.Create(c.Request.Context(), rt); err != nil {
		if errors.Is(err, storage.ErrRouteNameConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "route name already exists"})
			return
		}
		h.logger.Error().Err(err).Str("path", rt.Path).Msg("failed to create route")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().Str("route_id", rt.ID).Str("path", rt.Path).Msg("route created successfully")
	c.JSON(http.StatusCreated, gin.H{"route": rt})
}

func (h *RouteHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "route id is required"})
		return
	}

	h.logger.Info().Str("route_id", id).Msg("deleting route")

	err := h.repo.Delete(c.Request.Context(), id)
	if err == nil {
		h.logger.Info().Str("route_id", id).Msg("route deleted")
		c.JSON(http.StatusOK, gin.H{"message": "route deleted", "id": id})
		return
	}

	if errors.Is(err, storage.ErrRouteNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	h.logger.Error().Err(err).Str("route_id", id).Msg("failed to delete route")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
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

	if req.MatchType == "" {
		req.MatchType = "prefix"
	}
	if !validMatchTypes[req.MatchType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "match_type must be 'exact' or 'prefix'"})
		return
	}

	var policyID *string
	if req.PolicyID != nil && *req.PolicyID != "" {
		policyID = req.PolicyID
	}

	rt := &model.Route{
		Name:       req.Name,
		Path:       req.Path,
		UpstreamID: &req.UpstreamID,
		PolicyID:   policyID,
		Methods:    req.Methods,
		MatchType:  req.MatchType,
		Enabled:    req.Enabled,
	}

	h.logger.Info().
		Str("route_id", id).
		Str("path", rt.Path).
		Str("upstream_id", req.UpstreamID).
		Msg("updating route in database")

	if err := h.repo.Update(c.Request.Context(), id, rt); err != nil {
		if errors.Is(err, storage.ErrRouteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
			return
		}
		h.logger.Error().Err(err).Str("route_id", id).Msg("failed to update route")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().Str("route_id", rt.ID).Str("path", rt.Path).Msg("route updated successfully")
	c.JSON(http.StatusOK, gin.H{"route": rt})
}

func (h *RouteHandler) Disable(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "route id is required"})
		return
	}

	h.logger.Info().Str("route_id", id).Msg("disabling route")

	err := h.repo.Disable(c.Request.Context(), id)
	if err == nil {
		h.logger.Info().Str("route_id", id).Msg("route disabled successfully")
		c.JSON(http.StatusOK, gin.H{"message": "route disabled", "id": id})
		return
	}

	if errors.Is(err, storage.ErrRouteNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	h.logger.Error().Err(err).Str("route_id", id).Msg("failed to disable route")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func (h *RouteHandler) Enable(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "route id is required"})
		return
	}

	h.logger.Info().Str("route_id", id).Msg("enabling route")

	err := h.repo.Enable(c.Request.Context(), id)
	if err == nil {
		h.logger.Info().Str("route_id", id).Msg("route enabled successfully")
		c.JSON(http.StatusOK, gin.H{"message": "route enabled", "id": id})
		return
	}

	if errors.Is(err, storage.ErrRouteNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	h.logger.Error().Err(err).Str("route_id", id).Msg("failed to enable route")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

