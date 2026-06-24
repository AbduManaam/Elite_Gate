package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"elitegate/internal/model"
	"elitegate/internal/storage"
)

type PolicyHandler struct {
	policyRepo *storage.PolicyRepo
	routeRepo  *storage.RouteRepo
	logger     zerolog.Logger
}

func NewPolicyHandler(policyRepo *storage.PolicyRepo, routeRepo *storage.RouteRepo, logger zerolog.Logger) *PolicyHandler {
	return &PolicyHandler{policyRepo: policyRepo, routeRepo: routeRepo, logger: logger}
}

type policyRequest struct {
	Name           string   `json:"name" binding:"required"`
	AuthRequired   bool     `json:"auth_required"`
	RateLimitRPM   int      `json:"rate_limit_rpm"`
	AllowedOrigins []string `json:"allowed_origins"`
}

func (h *PolicyHandler) Create(c *gin.Context) {
	var req policyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.RateLimitRPM < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rate_limit_rpm must be >= 0"})
		return
	}

	p := &model.Policy{
		Name:           req.Name,
		AuthRequired:   req.AuthRequired,
		RateLimitRPM:   req.RateLimitRPM,
		AllowedOrigins: req.AllowedOrigins,
	}

	if err := h.policyRepo.Create(c.Request.Context(), p); err != nil {
		if errors.Is(err, storage.ErrPolicyNameConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "policy name already exists"})
			return
		}
		h.logger.Error().Err(err).Str("name", req.Name).Msg("failed to create policy")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"policy": p})
}

func (h *PolicyHandler) List(c *gin.Context) {
	policies, err := h.policyRepo.ListAll(c.Request.Context())
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to list policies")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"policies": policies})
}

func (h *PolicyHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.policyRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, storage.ErrPolicyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
			return
		}
		h.logger.Error().Err(err).Str("policy_id", id).Msg("failed to delete policy")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "policy deleted", "id": id})
}

func (h *PolicyHandler) Update(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "policy id is required"})
		return
	}

	var req policyRequest
	h.logger.Info().Str("policy_id", id).Msg("update policy request received")

	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error().Err(err).Msg("failed to parse update request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.RateLimitRPM < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rate_limit_rpm must be >= 0"})
		return
	}

	p := &model.Policy{
		Name:           req.Name,
		AuthRequired:   req.AuthRequired,
		RateLimitRPM:   req.RateLimitRPM,
		AllowedOrigins: req.AllowedOrigins,
	}

	tc, err := storage.TenantFromContext(c.Request.Context())
	if err == nil {
		p.ProjectID = tc.ProjectID.String()
	}

	h.logger.Info().
		Str("policy_id", id).
		Str("name", p.Name).
		Msg("updating policy in database")

	if err := h.policyRepo.Update(c.Request.Context(), id, p); err != nil {
		if errors.Is(err, storage.ErrPolicyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
			return
		}
		h.logger.Error().Err(err).Str("policy_id", id).Msg("failed to update policy")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	p.ID = id
	h.logger.Info().Str("policy_id", p.ID).Str("name", p.Name).Msg("policy updated successfully")
	c.JSON(http.StatusOK, gin.H{"policy": p})
}

type policyAssignRequest struct {
	PolicyID string `json:"policy_id" binding:"required"`
}

func (h *PolicyHandler) AssignPolicy(c *gin.Context) {
	routeID := c.Param("id")

	var req policyAssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify the policy actually exists in this project before attaching it —
	// otherwise the route update fails with a raw FK-constraint error.
	if _, err := h.policyRepo.GetByID(c.Request.Context(), req.PolicyID); err != nil {
		if errors.Is(err, storage.ErrPolicyNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "policy not found"})
			return
		}
		h.logger.Error().Err(err).Str("policy_id", req.PolicyID).Msg("failed to look up policy")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	route, err := h.routeRepo.GetByID(c.Request.Context(), routeID)
	if err != nil {
		if errors.Is(err, storage.ErrRouteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
			return
		}
		h.logger.Error().Err(err).Str("route_id", routeID).Msg("failed to look up route")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	route.PolicyID = &req.PolicyID

	if err := h.routeRepo.Update(c.Request.Context(), routeID, route); err != nil {
		if errors.Is(err, storage.ErrRouteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
			return
		}
		h.logger.Error().Err(err).Str("route_id", routeID).Str("policy_id", req.PolicyID).Msg("failed to assign policy")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().Str("route_id", routeID).Str("policy_id", req.PolicyID).Msg("policy assigned to route")
	c.JSON(http.StatusOK, gin.H{"message": "policy assigned successfully", "route": route})
}

func (h *PolicyHandler) RemovePolicy(c *gin.Context) {
	routeID := c.Param("id")

	route, err := h.routeRepo.GetByID(c.Request.Context(), routeID)
	if err != nil {
		if errors.Is(err, storage.ErrRouteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
			return
		}
		h.logger.Error().Err(err).Str("route_id", routeID).Msg("failed to look up route")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	route.PolicyID = nil

	if err := h.routeRepo.Update(c.Request.Context(), routeID, route); err != nil {
		if errors.Is(err, storage.ErrRouteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
			return
		}
		h.logger.Error().Err(err).Str("route_id", routeID).Msg("failed to remove policy")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().Str("route_id", routeID).Msg("policy removed from route")
	c.JSON(http.StatusOK, gin.H{"message": "policy removed successfully", "route": route})
}
