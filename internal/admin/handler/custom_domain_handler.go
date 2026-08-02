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

// Delete initiates asynchronous deprovisioning for a custom domain.
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

	deprovisioningDomain, err := h.svc.DeleteCustomDomain(
		c.Request.Context(),
		tenantContext.ProjectID,
		customDomainID,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAutomationDisabled):
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": err.Error(),
			})
			return

		case errors.Is(err, service.ErrCustomDomainNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"error": "custom domain not found",
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
				"failed to delete custom domain",
			)
			return
		}
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(
			c,
			"custom_domain.delete",
			"custom_domain",
			customDomainID.String(),
			deprovisioningDomain.Hostname,
			gin.H{
				"provisioning_status": deprovisioningDomain.ProvisioningStatus,
			},
		)
	}

	h.logger.Info().
		Str("custom_domain_id", customDomainID.String()).
		Str("project_id", tenantContext.ProjectID.String()).
		Str("provisioning_status", deprovisioningDomain.ProvisioningStatus).
		Msg("custom domain deprovisioning initiated")

	if deprovisioningDomain.ProvisioningStatus == domain.ProvisioningStatusDeprovisioned || deprovisioningDomain.DeletedAt != nil {
		c.JSON(http.StatusOK, gin.H{
			"message":       "custom domain is already deprovisioned",
			"custom_domain": deprovisioningDomain,
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":            "custom domain deprovisioning started",
		"status":             "deprovisioning",
		"provisioningStatus": deprovisioningDomain.ProvisioningStatus,
		"custom_domain":      deprovisioningDomain,
	})
}

// RetryDeprovisioning safely restarts custom domain deprovisioning after a failure.
//
// Route: POST /admin/v1/projects/:projectId/custom-domains/:domainId/retry-deprovisioning
func (h *CustomDomainHandler) RetryDeprovisioning(c *gin.Context) {
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

	retriedDomain, err := h.svc.RetryDeprovisioning(
		c.Request.Context(),
		tenantContext.ProjectID,
		customDomainID,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAutomationDisabled):
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": err.Error(),
			})
			return

		case errors.Is(err, service.ErrCustomDomainNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"error": "custom domain not found",
			})
			return

		case errors.Is(err, service.ErrDomainNotEligibleForRetry):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "custom domain is not eligible for deprovisioning retry",
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
				"failed to retry deprovisioning",
			)
			return
		}
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(
			c,
			"custom_domain.retry_deprovisioning",
			"custom_domain",
			retriedDomain.ID.String(),
			retriedDomain.Hostname,
			gin.H{
				"provisioning_status": retriedDomain.ProvisioningStatus,
			},
		)
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":            "custom domain deprovisioning retry initiated",
		"status":             "deprovisioning_restarted",
		"provisioningStatus": retriedDomain.ProvisioningStatus,
		"custom_domain":      retriedDomain,
	})
}

// CheckRouting verifies the CNAME record for a verified custom domain.
//
// Route: POST /admin/v1/projects/:projectId/custom-domains/:domainId/check-routing
func (h *CustomDomainHandler) CheckRouting(c *gin.Context) {
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

	verifiedDomain, err := h.svc.CheckCustomDomainRouting(
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

		case errors.Is(err, service.ErrCustomDomainNotVerified):
			c.JSON(http.StatusConflict, gin.H{
				"error": "custom domain ownership must be verified before checking routing",
			})
			return

		case errors.Is(err, service.ErrCNAMERecordNotFound):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "DNS CNAME record was not found",
			})
			return

		case errors.Is(err, service.ErrCNAMERoutingMismatch):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "CNAME record does not point to the expected gateway target",
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
				"failed to check custom domain routing",
			)
			return
		}
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(
			c,
			"custom_domain.check_routing",
			"custom_domain",
			verifiedDomain.ID.String(),
			verifiedDomain.Hostname,
			gin.H{
				"routing_status": verifiedDomain.RoutingStatus,
				"routing_target": verifiedDomain.RoutingTarget,
			},
		)
	}

	h.logger.Info().
		Str("custom_domain_id", verifiedDomain.ID.String()).
		Str("project_id", verifiedDomain.ProjectID.String()).
		Str("hostname", verifiedDomain.Hostname).
		Msg("custom domain CNAME routing verified")

	c.JSON(http.StatusOK, gin.H{
		"message":       "custom domain CNAME routing verified successfully",
		"custom_domain": verifiedDomain,
	})
}

// Activate promotes a verified and routing-ready custom domain to active status or enqueues async provisioning.
//
// Route: POST /admin/v1/projects/:projectId/custom-domains/:domainId/activate
func (h *CustomDomainHandler) Activate(c *gin.Context) {
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

	res, err := h.svc.ActivateCustomDomain(
		c.Request.Context(),
		tenantContext.ProjectID,
		customDomainID,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAutomationDisabled):
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": err.Error(),
			})
			return

		case errors.Is(err, service.ErrCustomDomainNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"error": "custom domain not found",
			})
			return

		case errors.Is(err, service.ErrCustomDomainNotVerified):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "custom domain must be verified before activation",
			})
			return

		case errors.Is(err, service.ErrCustomDomainRoutingNotReady):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "custom domain routing status must be ready before activation",
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
				"failed to activate custom domain",
			)
			return
		}
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(
			c,
			"custom_domain.activate",
			"custom_domain",
			res.Domain.ID.String(),
			res.Domain.Hostname,
			gin.H{
				"status":              res.Domain.Status,
				"routing_status":      res.Domain.RoutingStatus,
				"provisioning_status": res.Domain.ProvisioningStatus,
			},
		)
	}

	h.logger.Info().
		Str("custom_domain_id", res.Domain.ID.String()).
		Str("project_id", res.Domain.ProjectID.String()).
		Str("hostname", res.Domain.Hostname).
		Str("activation_state", string(res.State)).
		Msg("custom domain activation processed")

	switch res.State {
	case domain.ActivationAlreadyActive:
		c.JSON(http.StatusOK, gin.H{
			"message":       "custom domain is already active",
			"custom_domain": res.Domain,
		})
	case domain.ActivationInProgress:
		c.JSON(http.StatusAccepted, gin.H{
			"message":            "custom domain provisioning in progress",
			"status":             "provisioning_in_progress",
			"provisioningStatus": res.Domain.ProvisioningStatus,
			"custom_domain":      res.Domain,
		})
	default:
		c.JSON(http.StatusAccepted, gin.H{
			"message":            "custom domain provisioning initiated",
			"status":             "provisioning_started",
			"provisioningStatus": res.Domain.ProvisioningStatus,
			"custom_domain":      res.Domain,
		})
	}
}

// GetProvisioningStatus retrieves public provisioning status details for a domain.
//
// Route: GET /admin/v1/projects/:projectId/custom-domains/:domainId/provisioning-status
func (h *CustomDomainHandler) GetProvisioningStatus(c *gin.Context) {
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

	statusResp, err := h.svc.GetProvisioningStatus(
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
			"failed to get provisioning status",
		)
		return
	}

	c.JSON(http.StatusOK, statusResp)
}

// RetryProvisioning safely restarts custom domain provisioning after a terminal or transient failure.
//
// Route: POST /admin/v1/projects/:projectId/custom-domains/:domainId/retry-provisioning
func (h *CustomDomainHandler) RetryProvisioning(c *gin.Context) {
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

	retriedDomain, err := h.svc.RetryProvisioning(
		c.Request.Context(),
		tenantContext.ProjectID,
		customDomainID,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAutomationDisabled):
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": err.Error(),
			})
			return

		case errors.Is(err, service.ErrCustomDomainNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"error": "custom domain not found",
			})
			return

		case errors.Is(err, service.ErrDomainNotEligibleForRetry):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "custom domain is not eligible for retry",
			})
			return

		case errors.Is(err, service.ErrCustomDomainNotVerified):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "custom domain must be verified before retrying provisioning",
			})
			return

		case errors.Is(err, service.ErrCustomDomainRoutingNotReady):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "custom domain routing status must be ready before retrying provisioning",
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
				"failed to retry provisioning",
			)
			return
		}
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(
			c,
			"custom_domain.retry_provisioning",
			"custom_domain",
			retriedDomain.ID.String(),
			retriedDomain.Hostname,
			gin.H{
				"provisioning_status": retriedDomain.ProvisioningStatus,
			},
		)
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":            "custom domain provisioning retry initiated",
		"status":             "provisioning_restarted",
		"provisioningStatus": retriedDomain.ProvisioningStatus,
		"custom_domain":      retriedDomain,
	})
}
