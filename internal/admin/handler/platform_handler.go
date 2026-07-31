package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"elitegate/helper"
	adminmw "elitegate/internal/admin/middleware"
	"elitegate/internal/admin/service"
	"elitegate/internal/container"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type PlatformHandler struct {
	svc          *service.PlatformService
	gatewayRepo  *storage.GatewayRepo
	containerMgr container.ContainerManager
	syncHandler  *SyncHandler
	logger       zerolog.Logger
}

func NewPlatformHandler(
	svc *service.PlatformService,
	gatewayRepo *storage.GatewayRepo,
	containerMgr container.ContainerManager,
	syncHandler *SyncHandler,
	logger zerolog.Logger,
) *PlatformHandler {
	return &PlatformHandler{
		svc:          svc,
		gatewayRepo:  gatewayRepo,
		containerMgr: containerMgr,
		syncHandler:  syncHandler,
		logger:       logger.With().Str("handler", "platform").Logger(),
	}
}

func actingAdminID(c *gin.Context) string {
	val, exists := c.Get(adminmw.AdminUserIDKey)
	if !exists {
		return "unknown"
	}
	id, ok := val.(string)
	if !ok {
		return "unknown"
	}
	return id
}

func (h *PlatformHandler) ListTenants(c *gin.Context) {
	page, limit, offset, err := service.ParsePaginationOffset(c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	projects, total, err := h.svc.ListTenants(c.Request.Context(), limit, offset)
	if err != nil {
		helper.RespondInternalError(c, h.logger, err, "failed to list tenants")
		return
	}

	h.logger.Info().Int("count", len(projects)).Msg("platform-wide tenant list fetched")

	c.JSON(http.StatusOK, model.PaginatedResponse[model.Project]{
		Items:      projects,
		Pagination: service.BuildPagination(page, limit, total),
	})
}

func (h *PlatformHandler) DeleteTenant(c *gin.Context) {
	projectID := c.Param("projectId")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project id is required"})
		return
	}

	adminID := actingAdminID(c)

	if err := h.svc.DeleteTenant(c.Request.Context(), projectID); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("project_id", projectID).Str("acting_admin_id", adminID).Logger(), err, "failed to delete tenant")
		return
	}

	h.logger.Info().
		Str("project_id", projectID).
		Str("acting_admin_id", adminID).
		Msg("platform override: tenant deleted by super-admin")

	c.JSON(http.StatusOK, gin.H{
		"message":    "tenant deleted",
		"project_id": projectID,
	})
}

type gatewayHealthResult struct {
	GatewayID string `json:"gateway_id"`
	Status    string `json:"status"` // "healthy" | "unreachable"
}

func (h *PlatformHandler) PlatformHealth(c *gin.Context) {
	ctx := c.Request.Context()
	adminID := actingAdminID(c)

	projectCounts, gatewayCounts, err := h.svc.GetPlatformHealthCounts(ctx)
	if err != nil {
		helper.RespondInternalError(c, h.logger.With().Str("acting_admin_id", adminID).Logger(), err, "failed to compute platform health")
		return
	}

	activeGateways, err := h.gatewayRepo.ListActive(ctx)
	if err != nil {
		helper.RespondInternalError(c, h.logger.With().Str("acting_admin_id", adminID).Logger(), err, "failed to list active gateways for health probe")
		return
	}

	probeResults := h.probeActiveGateways(ctx, activeGateways)

	c.JSON(http.StatusOK, gin.H{
		"projects":       projectCounts,
		"gateways":       gatewayCounts,
		"gateway_probes": probeResults,
	})
}

func (h *PlatformHandler) probeActiveGateways(ctx context.Context, gateways []storage.GatewayRecord) []gatewayHealthResult {
	if len(gateways) == 0 {
		return []gatewayHealthResult{}
	}

	results := make([]gatewayHealthResult, len(gateways))
	var wg sync.WaitGroup

	for i, g := range gateways {
		wg.Add(1)
		go func(idx int, gw storage.GatewayRecord) {
			defer wg.Done()

			probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			url := fmt.Sprintf("http://%s:%s/healthz", gw.EndpointIP, gw.Port)
			req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)

			status := "unreachable"
			if err == nil {
				client := &http.Client{Timeout: 2 * time.Second}
				resp, err := client.Do(req)
				if err == nil {
					_ = resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						status = "healthy"
					}
				}
			}

			results[idx] = gatewayHealthResult{
				GatewayID: gw.ID,
				Status:    status,
			}
		}(i, g)
	}

	wg.Wait()
	return results
}

func (h *PlatformHandler) PlatformMetrics(c *gin.Context) {
	ctx := c.Request.Context()
	adminID := actingAdminID(c)

	projectCounts, gatewayCounts, adminUsers, err := h.svc.GetPlatformMetricsCounts(ctx)
	if err != nil {
		helper.RespondInternalError(c, h.logger.With().Str("acting_admin_id", adminID).Logger(), err, "failed to collect platform metrics")
		return
	}

	activeGateways, _ := h.gatewayRepo.ListActive(ctx)

	c.JSON(http.StatusOK, gin.H{
		"total_projects":        projectCounts.Total,
		"total_active_projects": projectCounts.Active,
		"total_gateways":        gatewayCounts["total"],
		"total_active_gateways": gatewayCounts["active"],
		"active_gateway_nodes":  len(activeGateways),
		"total_admin_users":     adminUsers,
		"timestamp":             time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *PlatformHandler) SuspendTenant(c *gin.Context) {
	projectID := c.Param("projectId")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project id is required"})
		return
	}

	adminID := actingAdminID(c)

	if err := h.svc.SuspendTenant(c.Request.Context(), projectID); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("project_id", projectID).Str("acting_admin_id", adminID).Logger(), err, "failed to suspend tenant")
		return
	}

	h.logger.Info().
		Str("project_id", projectID).
		Str("acting_admin_id", adminID).
		Msg("platform override: tenant suspended by super-admin")

	c.JSON(http.StatusOK, gin.H{
		"message":    "tenant suspended",
		"project_id": projectID,
		"status":     "suspended",
	})
}

func (h *PlatformHandler) ReactivateTenant(c *gin.Context) {
	projectID := c.Param("projectId")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project id is required"})
		return
	}

	adminID := actingAdminID(c)

	if err := h.svc.ReactivateTenant(c.Request.Context(), projectID); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("project_id", projectID).Str("acting_admin_id", adminID).Logger(), err, "failed to reactivate tenant")
		return
	}

	h.logger.Info().
		Str("project_id", projectID).
		Str("acting_admin_id", adminID).
		Msg("platform override: tenant reactivated by super-admin")

	c.JSON(http.StatusOK, gin.H{
		"message":    "tenant reactivated",
		"project_id": projectID,
		"status":     "active",
	})
}

func (h *PlatformHandler) RestartGateway(c *gin.Context) {
	gatewayID := c.Param("gatewayId")
	if gatewayID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway id is required"})
		return
	}

	ctx := c.Request.Context()
	adminID := actingAdminID(c)

	gw, err := h.gatewayRepo.GetByExternalID(ctx, gatewayID)
	if err != nil {
		if errors.Is(err, storage.ErrGatewayNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "gateway not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("gateway_id", gatewayID).Str("acting_admin_id", adminID).Logger(), err, "failed to lookup gateway")
		return
	}

	if h.syncHandler == nil {
		helper.RespondInternalError(c, h.logger.With().Str("gateway_id", gatewayID).Str("acting_admin_id", adminID).Logger(), nil, "sync handler not wired into platform handler")
		return
	}

	if err := h.syncHandler.reloadOne(ctx, *gw); err != nil {
		h.logger.Warn().
			Err(err).
			Str("gateway_id", gatewayID).
			Str("acting_admin_id", adminID).
			Msg("restart gateway failed")

		c.JSON(http.StatusBadGateway, gin.H{
			"error":      "failed to trigger config reload on gateway node",
			"gateway_id": gatewayID,
			"details":    err.Error(),
		})
		return
	}

	h.logger.Info().
		Str("gateway_id", gatewayID).
		Str("acting_admin_id", adminID).
		Msg("platform override: gateway container reloaded")

	c.JSON(http.StatusOK, gin.H{
		"message":    "gateway config reload triggered successfully",
		"gateway_id": gatewayID,
	})
}

func (h *PlatformHandler) ForceDecommission(c *gin.Context) {
	gatewayID := c.Param("gatewayId")
	if gatewayID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway id is required"})
		return
	}

	ctx := c.Request.Context()
	adminID := actingAdminID(c)

	if h.containerMgr != nil {
		if err := h.containerMgr.Decommission(ctx, gatewayID); err != nil {
			h.logger.Warn().
				Err(err).
				Str("gateway_id", gatewayID).
				Msg("failed to remove container during force decommission (continuing to mark DB record)")
		}
	}

	if err := h.gatewayRepo.UpdateStatus(ctx, gatewayID, "decommissioned"); err != nil {
		if errors.Is(err, storage.ErrGatewayNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "gateway not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("gateway_id", gatewayID).Str("acting_admin_id", adminID).Logger(), err, "failed to update gateway status in database")
		return
	}

	h.logger.Info().
		Str("gateway_id", gatewayID).
		Str("acting_admin_id", adminID).
		Msg("platform override: gateway force-decommissioned by super-admin")

	c.JSON(http.StatusOK, gin.H{
		"message":    "gateway force-decommissioned",
		"gateway_id": gatewayID,
		"status":     "decommissioned",
	})
}
