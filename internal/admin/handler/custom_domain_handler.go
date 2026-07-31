package handler

import (
	"errors"
	"net/http"

	"elitegate/helper"
	"elitegate/internal/admin/service"
	"elitegate/internal/domain"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// CustomDomainHandler handles custom-domain HTTP requests.
type CustomDomainHandler struct {
	svc      *service.CustomDomainService
	auditSvc *service.AuditService
	logger   zerolog.Logger
}

// NewCustomDomainHandler creates a CustomDomainHandler.
func NewCustomDomainHandler(
	svc *service.CustomDomainService,
	logger zerolog.Logger,
	auditSvc *service.AuditService,
) *CustomDomainHandler {
	return &CustomDomainHandler{
		svc:      svc,
		auditSvc: auditSvc,
		logger: logger.With().
			Str("handler", "custom_domain").
			Logger(),
	}
}

// Create registers a custom domain for the project in the current tenant
// context.
//
// Route:
//
//	POST /admin/v1/projects/:projectId/custom-domains
func (h *CustomDomainHandler) Create(c *gin.Context) {
	var req domain.CreateCustomDomainRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "hostname is required",
		})
		return
	}

	tenantValue, exists := c.Get("tenant_ctx")
	if !exists {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "tenant context missing",
		})
		return
	}

	tenantContext, ok := tenantValue.(storage.TenantContext)
	if !ok {
		helper.RespondInternalError(
			c,
			h.logger,
			nil,
			"invalid tenant context",
		)
		return
	}

	result, err := h.svc.CreateCustomDomain(
		c.Request.Context(),
		tenantContext.ProjectID,
		req.Hostname,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCustomDomainHostname):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return

		case errors.Is(err, service.ErrCustomDomainAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{
				"error": "custom domain is already registered",
			})
			return

		default:
			helper.RespondInternalError(
				c,
				h.logger.With().
					Str("project_id", tenantContext.ProjectID.String()).
					Str("hostname", req.Hostname).
					Logger(),
				err,
				"failed to register custom domain",
			)
			return
		}
	}

	h.logger.Info().
		Str("custom_domain_id", result.ID.String()).
		Str("project_id", result.ProjectID.String()).
		Str("hostname", result.Hostname).
		Msg("custom domain registered")

	if h.auditSvc != nil {
		h.auditSvc.Record(
			c,
			"custom_domain.create",
			"custom_domain",
			result.ID.String(),
			result.Hostname,
			gin.H{
				"hostname": result.Hostname,
				"status":   result.Status,
			},
		)
	}

	c.JSON(http.StatusCreated, gin.H{
		"custom_domain": result,
	})
}

// Verify checks the customer's DNS TXT record and verifies domain ownership.
func (h *CustomDomainHandler) Verify(c *gin.Context) {
	tenantContextValue, exists := c.Get("tenant_ctx")
	if !exists {
		helper.RespondInternalError(
			c,
			h.logger,
			nil,
			"internal tenant context missing",
		)
		return
	}

	tenantContext, ok := tenantContextValue.(storage.TenantContext)
	if !ok {
		helper.RespondInternalError(
			c,
			h.logger,
			nil,
			"invalid tenant context",
		)
		return
	}

	customDomainID, err := uuid.Parse(c.Param("domainId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid custom domain ID",
		})
		return
	}

	verifiedDomain, err := h.svc.VerifyCustomDomain(
		c.Request.Context(),
		tenantContext.ProjectID,
		customDomainID,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCustomDomainNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"error": "custom domain not found",
			})
			return

		case errors.Is(err, service.ErrVerificationRecordNotFound):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "DNS verification TXT record was not found",
				"details": gin.H{
					"record_name": "_elitegate-verification",
					"record_type": "TXT",
				},
			})
			return

		case errors.Is(err, service.ErrVerificationTokenMismatch):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "DNS verification token does not match",
			})
			return

		default:
			helper.RespondInternalError(
				c,
				h.logger.With().
					Str("custom_domain_id", customDomainID.String()).
					Str("project_id", tenantContext.ProjectID.String()).
					Logger(),
				err,
				"failed to verify custom domain",
			)
			return
		}
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(
			c,
			"custom_domain.verify",
			"custom_domain",
			verifiedDomain.ID.String(),
			verifiedDomain.Hostname,
			gin.H{
				"status": verifiedDomain.Status,
			},
		)
	}

	h.logger.Info().
		Str("custom_domain_id", verifiedDomain.ID.String()).
		Str("project_id", verifiedDomain.ProjectID.String()).
		Str("hostname", verifiedDomain.Hostname).
		Msg("custom domain verified")

	c.JSON(http.StatusOK, gin.H{
		"message":       "custom domain verified successfully",
		"custom_domain": verifiedDomain,
	})
}

// getTenantContext extracts and validates the tenant context from Gin context.
func (h *CustomDomainHandler) getTenantContext(c *gin.Context) (storage.TenantContext, bool) {
	tenantContextValue, exists := c.Get("tenant_ctx")
	if !exists {
		helper.RespondInternalError(
			c,
			h.logger,
			nil,
			"internal tenant context missing",
		)
		return storage.TenantContext{}, false
	}

	tenantContext, ok := tenantContextValue.(storage.TenantContext)
	if !ok {
		helper.RespondInternalError(
			c,
			h.logger,
			nil,
			"invalid tenant context",
		)
		return storage.TenantContext{}, false
	}

	return tenantContext, true
}

// List returns all custom domains for the project in the current tenant context.
//
// Route: GET /admin/v1/projects/:projectId/custom-domains
func (h *CustomDomainHandler) List(c *gin.Context) {
	tenantContext, ok := h.getTenantContext(c)
	if !ok {
		return
	}

	domains, err := h.svc.ListCustomDomains(
		c.Request.Context(),
		tenantContext.ProjectID,
	)
	if err != nil {
		helper.RespondInternalError(
			c,
			h.logger.With().
				Str("project_id", tenantContext.ProjectID.String()).
				Logger(),
			err,
			"failed to list custom domains",
		)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"custom_domains": domains,
	})
}

// Get returns a single custom domain by ID for the project in the current tenant context.
//
// Route: GET /admin/v1/projects/:projectId/custom-domains/:domainId
func (h *CustomDomainHandler) Get(c *gin.Context) {
	tenantContext, ok := h.getTenantContext(c)
	if !ok {
		return
	}

	customDomainID, err := uuid.Parse(c.Param("domainId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid custom domain ID",
		})
		return
	}

	customDomain, err := h.svc.GetCustomDomain(
		c.Request.Context(),
		tenantContext.ProjectID,
		customDomainID,
	)
	if err != nil {
		if errors.Is(err, service.ErrCustomDomainNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "custom domain not found",
			})
			return
		}

		helper.RespondInternalError(
			c,
			h.logger.With().
				Str("custom_domain_id", customDomainID.String()).
				Str("project_id", tenantContext.ProjectID.String()).
				Logger(),
			err,
			"failed to get custom domain",
		)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"custom_domain": customDomain,
	})
}

// Delete soft-deletes a custom domain for the project in the current tenant context.
//
// Route: DELETE /admin/v1/projects/:projectId/custom-domains/:domainId
func (h *CustomDomainHandler) Delete(c *gin.Context) {
	tenantContext, ok := h.getTenantContext(c)
	if !ok {
		return
	}

	customDomainID, err := uuid.Parse(c.Param("domainId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid custom domain ID",
		})
		return
	}

	err = h.svc.DeleteCustomDomain(
		c.Request.Context(),
		tenantContext.ProjectID,
		customDomainID,
	)
	if err != nil {
		if errors.Is(err, service.ErrCustomDomainNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "custom domain not found",
			})
			return
		}

		helper.RespondInternalError(
			c,
			h.logger.With().
				Str("custom_domain_id", customDomainID.String()).
				Str("project_id", tenantContext.ProjectID.String()).
				Logger(),
			err,
			"failed to delete custom domain",
		)
		return
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(
			c,
			"custom_domain.delete",
			"custom_domain",
			customDomainID.String(),
			"",
			nil,
		)
	}

	h.logger.Info().
		Str("custom_domain_id", customDomainID.String()).
		Str("project_id", tenantContext.ProjectID.String()).
		Msg("custom domain deleted")

	c.JSON(http.StatusOK, gin.H{
		"message": "custom domain deleted successfully",
		"id":      customDomainID.String(),
	})
}
