package handler

import (
	"elitegate/internal/model"
	"elitegate/internal/storage"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type RouteHandler struct {
	repo *storage.RouteRepo
	logger zerolog.Logger
}

func NewRouteHandler(repo *storage.RouteRepo, logger zerolog.Logger)*RouteHandler{
	return &RouteHandler{
		repo: repo,
		logger: logger,
	}
}

func(h *RouteHandler)List(c *gin.Context){
	
	routes,err:= h.repo.ListAll(c.Request.Context())
	if err!=nil{
       h.logger.Debug().
	   Err(err).
	   Str("handler", "ListRoutes").
	   Msg("failed to list routes")

		c.JSON(http.StatusInternalServerError,gin.H{"error": "failed to load routes"})
		return
	}
	c.JSON(http.StatusOK,gin.H{"routes":routes})
}

type createRouteRequest struct {
	Path         string   `json:"path" binding:"required"`
	UpstreamURL  string   `json:"upstream_url" binding:"required"`
	Methods      []string `json:"methods" binding:"required"`
	Protocol     string   `json:"protocol"`
	MatchType    string   `json:"match_type"`
	Enabled      bool     `json:"enabled"`
	AuthRequired bool     `json:"auth_required"`
	RateLimitRPM int      `json:"rate_limit_rpm"`
}

var validProtocols = map[string]bool{"http": true, "grpc": true}
var validMatchTypes = map[string]bool{"exact": true, "prefix": true}


func (h *RouteHandler) Create(c *gin.Context) {
	var req createRouteRequest

    h.logger.Info().
	Str("method",c.Request.Method).
	Str("path",c.Request.URL.Path).
	Msg("create route request received")

	// Parse JSON
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error().
			Err(err).
			Msg("failed to parse request body")

		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Debug: Show parsed request
	h.logger.Info().
		Interface("request", req).
		Msg("request parsed successfully")
 
	if req.Protocol == "" {
		req.Protocol = "http"
	}
	if req.MatchType == "" {
		req.MatchType = "prefix"
	}
 
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

	// Validate match type
	if !validMatchTypes[req.MatchType] {
		h.logger.Warn().
			Str("match_type", req.MatchType).
			Msg("invalid match type")

		c.JSON(http.StatusBadRequest, gin.H{
			"error": "match_type must be 'exact' or 'prefix'",
		})
		return
	}
 
	rt := &model.Route{
		Path:         req.Path,
		UpstreamURL:  req.UpstreamURL,
		Methods:      req.Methods,
		Protocol:     req.Protocol,
		MatchType:    req.MatchType,
		Enabled:      req.Enabled,
		AuthRequired: req.AuthRequired,
		RateLimitRPM: req.RateLimitRPM,
	}

	h.logger.Info().
		Str("path", rt.Path).
		Str("upstream", rt.UpstreamURL).
		Str("protocol", rt.Protocol).
		Msg("creating route in database")

// Save to database
	if err := h.repo.Create(c.Request.Context(), rt); err != nil {
		h.logger.Error().
			Err(err).
			Str("path", rt.Path).
			Msg("failed to create route")

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Success log
	h.logger.Info().
		Str("route_id", rt.ID).
		Str("path", rt.Path).
		Msg("route created successfully")

	c.JSON(http.StatusCreated, gin.H{
		"route": rt,
	})
}

func (h *RouteHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "route id is required",
		})
		return
	}

	h.logger.Info().
		Str("route_id", id).
		Msg("deleting route")

	err := h.repo.Delete(c.Request.Context(), id)

	if err == nil {
		h.logger.Info().
			Str("route_id", id).
			Msg("route deleted")

		c.JSON(http.StatusOK, gin.H{
			"message": "route deleted",
			"id":      id,
		})
		return
	}

	if errors.Is(err, storage.ErrRouteNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "route not found",
		})
		return
	}

	h.logger.Error().
		Err(err).
		Str("route_id", id).
		Msg("failed to delete route")

	c.JSON(http.StatusInternalServerError, gin.H{
		"error": "internal server error",
	})
}