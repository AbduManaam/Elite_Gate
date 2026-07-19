package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"elitegate/helper"
	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"
)

type PolicyHandler struct {
	svc      *service.PolicyService
	auditSvc *service.AuditService
	logger   zerolog.Logger
}

func NewPolicyHandler(svc *service.PolicyService, logger zerolog.Logger, auditSvc *service.AuditService) *PolicyHandler {
	return &PolicyHandler{svc: svc, auditSvc: auditSvc, logger: logger}
}

type policyRequest struct {
	Name           string   `json:"name" binding:"required"`
	AuthRequired   bool     `json:"auth_required"`
	RateLimitRPM   int      `json:"rate_limit_rpm"`
	AllowedOrigins []string `json:"allowed_origins"`
	AllowedRoles   []string `json:"allowed_roles"`
	AllowedScopes  []string `json:"allowed_scopes"`
	IPAllowlist    []string `json:"ip_allowlist"`
	IPBlocklist    []string `json:"ip_blocklist"`
}

func (h *PolicyHandler) Create(c *gin.Context) {
	var req policyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	p, err := h.svc.CreatePolicy(c.Request.Context(), req.Name, req.AuthRequired, req.RateLimitRPM, req.AllowedOrigins, req.AllowedRoles, req.AllowedScopes, req.IPAllowlist, req.IPBlocklist)
	if err != nil {
		if errors.Is(err, storage.ErrPolicyNameConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "policy name already exists"})
			return
		}
		if err.Error() == "rate_limit_rpm must be >= 0" || errors.Is(err, service.ErrInvalidProtocol) || (len(err.Error()) > 18 && err.Error()[:18] == "invalid IP or CIDR") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("name", req.Name).Logger(), err, "internal server error")
		return
	}

	h.auditSvc.Record(c, "policy.create", "policy", p.ID, p.Name, gin.H{
		"name":            p.Name,
		"auth_required":   p.AuthRequired,
		"rate_limit_rpm":  p.RateLimitRPM,
		"allowed_origins": p.AllowedOrigins,
		"allowed_roles":   p.AllowedRoles,
		"allowed_scopes":  p.AllowedScopes,
		"ip_allowlist":    p.IPAllowlist,
		"ip_blocklist":    p.IPBlocklist,
	})

	c.JSON(http.StatusCreated, gin.H{"policy": p})
}

func (h *PolicyHandler) List(c *gin.Context) {
	page, limit, offset, err := service.ParsePaginationOffset(c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	policies, total, err := h.svc.ListPolicies(c.Request.Context(), limit, offset)
	if err != nil {
		helper.RespondInternalError(c, h.logger, err, "failed to load policies")
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse[model.Policy]{
		Items:      policies,
		Pagination: service.BuildPagination(page, limit, total),
	})
}

func (h *PolicyHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.svc.DeletePolicy(c.Request.Context(), id); err != nil {
		if errors.Is(err, storage.ErrPolicyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
			return
		}
		if errors.Is(err, storage.ErrPolicyInUse) {
			c.JSON(http.StatusConflict, gin.H{"error": "policy is attached to active routes and cannot be deleted"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("policy_id", id).Logger(), err, "internal server error")
		return
	}

	h.auditSvc.Record(c, "policy.delete", "policy", id, "", nil)
	c.JSON(http.StatusOK, gin.H{"message": "policy deleted", "id": id})
}

func (h *PolicyHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req policyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	p, err := h.svc.UpdatePolicy(c.Request.Context(), id, req.Name, req.AuthRequired, req.RateLimitRPM, req.AllowedOrigins, req.AllowedRoles, req.AllowedScopes, req.IPAllowlist, req.IPBlocklist)
	if err != nil {
		if errors.Is(err, storage.ErrPolicyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
			return
		}
		if err.Error() == "rate_limit_rpm must be >= 0" || (len(err.Error()) > 18 && err.Error()[:18] == "invalid IP or CIDR") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("policy_id", id).Logger(), err, "internal server error")
		return
	}

	h.auditSvc.Record(c, "policy.update", "policy", id, p.Name, gin.H{
		"name":            p.Name,
		"auth_required":   p.AuthRequired,
		"rate_limit_rpm":  p.RateLimitRPM,
		"allowed_origins": p.AllowedOrigins,
		"allowed_roles":   p.AllowedRoles,
		"allowed_scopes":  p.AllowedScopes,
		"ip_allowlist":    p.IPAllowlist,
		"ip_blocklist":    p.IPBlocklist,
	})

	c.JSON(http.StatusOK, gin.H{"policy": p})
}

type assignPolicyRequest struct {
	PolicyID string `json:"policy_id" binding:"required"`
}

func (h *PolicyHandler) AssignPolicy(c *gin.Context) {
	routeID := c.Param("id")

	var req assignPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.AssignPolicy(c.Request.Context(), routeID, req.PolicyID); err != nil {
		if errors.Is(err, storage.ErrRouteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
			return
		}
		if errors.Is(err, storage.ErrPolicyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("route_id", routeID).Str("policy_id", req.PolicyID).Logger(), err, "internal server error")
		return
	}

	h.auditSvc.Record(c, "route.update", "route", routeID, "", gin.H{"action": "assign_policy", "policy_id": req.PolicyID})

	c.JSON(http.StatusOK, gin.H{"message": "policy assigned to route", "route_id": routeID, "policy_id": req.PolicyID})
}

func (h *PolicyHandler) RemovePolicy(c *gin.Context) {
	routeID := c.Param("id")

	if err := h.svc.RemovePolicy(c.Request.Context(), routeID); err != nil {
		if errors.Is(err, storage.ErrRouteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("route_id", routeID).Logger(), err, "internal server error")
		return
	}

	h.auditSvc.Record(c, "route.update", "route", routeID, "", gin.H{"action": "remove_policy"})

	c.JSON(http.StatusOK, gin.H{"message": "policy removed from route", "route_id": routeID})
}
