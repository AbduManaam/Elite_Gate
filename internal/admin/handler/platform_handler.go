package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	adminmw "elitegate/internal/admin/middleware"
	"elitegate/internal/container"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// PlatformHandler serves cross-tenant, platform-operator-only endpoints
// under /admin/v1/platform/*. Every route using this handler MUST be
// gated by SuperAdminOnly middleware at the router level — this handler
// performs no per-request authorization check of its own, by design,
// to keep the authorization decision in exactly one place.
type PlatformHandler struct {
	projectRepo  *storage.ProjectRepo
	gatewayRepo  *storage.GatewayRepo
	authRepo     *storage.AdminAuthRepo
	containerMgr container.ContainerManager
	syncHandler  *SyncHandler
	logger       zerolog.Logger
}

func NewPlatformHandler(
	projectRepo *storage.ProjectRepo,
	gatewayRepo *storage.GatewayRepo,
	authRepo *storage.AdminAuthRepo,
	containerMgr container.ContainerManager,
	syncHandler *SyncHandler,
	logger zerolog.Logger,
) *PlatformHandler {
	return &PlatformHandler{
		projectRepo:  projectRepo,
		gatewayRepo:  gatewayRepo,
		authRepo:     authRepo,
		containerMgr: containerMgr,
		syncHandler:  syncHandler,
		logger:       logger.With().Str("handler", "platform").Logger(),
	}
}

// actingAdminID extracts the calling super-admin's user ID from context,
// set earlier by the AdminAuth middleware. Used to attribute every
// state-changing platform action in logs.
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

// ─────────────────────────────────────────────────────────────────────────────
// Item 1 — List All Tenants
// ─────────────────────────────────────────────────────────────────────────────

// ListTenants handles GET /admin/v1/platform/projects
func (h *PlatformHandler) ListTenants(c *gin.Context) {
	projects, err := h.projectRepo.ListAllGlobal(c.Request.Context())
	if err != nil {
		h.logger.Error().Err(err).
			Str("acting_admin_id", actingAdminID(c)).
			Msg("failed to list all tenants")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tenants"})
		return
	}

	h.logger.Debug().
		Str("acting_admin_id", actingAdminID(c)).
		Int("count", len(projects)).
		Msg("platform-wide tenant list fetched")

	c.JSON(http.StatusOK, gin.H{"projects": projects, "count": len(projects)})
}

// ─────────────────────────────────────────────────────────────────────────────
// Item 2 — Delete Any Tenant (Platform Override)
// ─────────────────────────────────────────────────────────────────────────────

// DeleteTenant handles DELETE /admin/v1/platform/projects/:projectId
//
// Operator override — bypasses the normal requirement that only a project's
// own owner can delete it. Reuses ProjectRepo.Delete unchanged (already
// cascades correctly to api_keys, routes, and upstreams in one transaction).
func (h *PlatformHandler) DeleteTenant(c *gin.Context) {
	projectID := c.Param("projectId")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project id is required"})
		return
	}

	adminID := actingAdminID(c)

	if err := h.projectRepo.Delete(c.Request.Context(), projectID); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		h.logger.Error().Err(err).
			Str("project_id", projectID).
			Str("acting_admin_id", adminID).
			Msg("platform override: failed to delete tenant")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete tenant"})
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

// ─────────────────────────────────────────────────────────────────────────────
// Item 3 — Platform-Wide Health
// ─────────────────────────────────────────────────────────────────────────────

// gatewayHealthResult is one entry in the platform health fan-out response.
type gatewayHealthResult struct {
	GatewayID string `json:"gateway_id"`
	Status    string `json:"status"` // "healthy" | "unreachable"
}

// PlatformHealth handles GET /admin/v1/platform/health
//
// Aggregates project activation counts and gateway status counts directly
// from Postgres, then fans out a lightweight HTTP health check to every
// active gateway's /health endpoint. A 2s per-gateway timeout prevents
// one slow/dead gateway from blocking the whole response.
func (h *PlatformHandler) PlatformHealth(c *gin.Context) {
	ctx := c.Request.Context()
	adminID := actingAdminID(c)

	projectCounts, err := h.projectRepo.GlobalCounts(ctx)
	if err != nil {
		h.logger.Error().Err(err).
			Str("acting_admin_id", adminID).
			Msg("platform health: failed to get project counts")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute platform health"})
		return
	}

	gatewayCounts, err := h.gatewayRepo.CountByStatus(ctx)
	if err != nil {
		h.logger.Error().Err(err).
			Str("acting_admin_id", adminID).
			Msg("platform health: failed to get gateway counts")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute platform health"})
		return
	}

	activeGateways, err := h.gatewayRepo.ListActive(ctx)
	if err != nil {
		h.logger.Error().Err(err).
			Str("acting_admin_id", adminID).
			Msg("platform health: failed to list active gateways for probing")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute platform health"})
		return
	}

	results := h.probeGateways(ctx, activeGateways)

	h.logger.Debug().
		Str("acting_admin_id", adminID).
		Int("active_gateways_probed", len(results)).
		Msg("platform health computed")

	c.JSON(http.StatusOK, gin.H{
		"projects":       projectCounts,
		"gateways":       gatewayCounts,
		"gateway_health": results,
	})
}

// probeGateways concurrently checks each gateway's /health endpoint.
// Failures are reported as "unreachable" — one dead gateway never
// fails the entire platform health view.
func (h *PlatformHandler) probeGateways(ctx context.Context, gateways []storage.GatewayRecord) []gatewayHealthResult {
	results := make([]gatewayHealthResult, len(gateways))
	var wg sync.WaitGroup

	client := &http.Client{Timeout: 2 * time.Second}

	for i, g := range gateways {
		wg.Add(1)
		go func(idx int, gw storage.GatewayRecord) {
			defer wg.Done()

			host := gw.EndpointIP
			if host == "" || host == "0.0.0.0" {
				host = "localhost"
			}
			url := fmt.Sprintf("http://%s:%s/health", host, gw.Port)

			reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
			if err != nil {
				results[idx] = gatewayHealthResult{GatewayID: gw.ExternalID, Status: "unreachable"}
				return
			}

			resp, err := client.Do(req)
			if err != nil {
				results[idx] = gatewayHealthResult{GatewayID: gw.ExternalID, Status: "unreachable"}
				return
			}
			defer resp.Body.Close()

			status := "unreachable"
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				status = "healthy"
			}
			results[idx] = gatewayHealthResult{GatewayID: gw.ExternalID, Status: status}
		}(i, g)
	}

	wg.Wait()
	return results
}

// ─────────────────────────────────────────────────────────────────────────────
// Item 4 — Platform-Wide Metrics
// ─────────────────────────────────────────────────────────────────────────────

// PlatformMetrics handles GET /admin/v1/platform/metrics
//
// Pure COUNT(*) aggregation over existing tables — no new metrics
// collection. Each count is a cheap indexed query.
func (h *PlatformHandler) PlatformMetrics(c *gin.Context) {
	ctx := c.Request.Context()
	adminID := actingAdminID(c)

	m, err := h.projectRepo.PlatformMetricsSnapshot(ctx)
	if err != nil {
		h.logger.Error().Err(err).
			Str("acting_admin_id", adminID).
			Msg("failed to compute platform metrics")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute platform metrics"})
		return
	}

	h.logger.Debug().Str("acting_admin_id", adminID).Msg("platform metrics computed")
	c.JSON(http.StatusOK, m)
}

// ─────────────────────────────────────────────────────────────────────────────
// Item 5 — Suspend / Reactivate a Tenant
// ─────────────────────────────────────────────────────────────────────────────

// SuspendTenant handles PATCH /admin/v1/platform/projects/:projectId/suspend
func (h *PlatformHandler) SuspendTenant(c *gin.Context) {
	projectID := c.Param("projectId")
	adminID := actingAdminID(c)

	if err := h.projectRepo.Suspend(c.Request.Context(), projectID); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		h.logger.Error().Err(err).
			Str("project_id", projectID).
			Str("acting_admin_id", adminID).
			Msg("failed to suspend tenant")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to suspend tenant"})
		return
	}

	h.logger.Info().
		Str("project_id", projectID).
		Str("acting_admin_id", adminID).
		Msg("tenant suspended by super-admin")

	c.JSON(http.StatusOK, gin.H{"message": "tenant suspended", "project_id": projectID})
}

// ReactivateTenant handles PATCH /admin/v1/platform/projects/:projectId/reactivate
func (h *PlatformHandler) ReactivateTenant(c *gin.Context) {
	projectID := c.Param("projectId")
	adminID := actingAdminID(c)

	if err := h.projectRepo.Reactivate(c.Request.Context(), projectID); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		h.logger.Error().Err(err).
			Str("project_id", projectID).
			Str("acting_admin_id", adminID).
			Msg("failed to reactivate tenant")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reactivate tenant"})
		return
	}

	h.logger.Info().
		Str("project_id", projectID).
		Str("acting_admin_id", adminID).
		Msg("tenant reactivated by super-admin")

	c.JSON(http.StatusOK, gin.H{"message": "tenant reactivated", "project_id": projectID})
}

// ─────────────────────────────────────────────────────────────────────────────
// Item 6 — Restart a Specific Gateway
// ─────────────────────────────────────────────────────────────────────────────

// RestartGateway handles POST /admin/v1/platform/gateways/:gatewayId/restart
//
// Looks up a single gateway by its external ID and triggers a config
// reload using the same logic SyncHandler.Reload uses in bulk.
func (h *PlatformHandler) RestartGateway(c *gin.Context) {
	gatewayID := c.Param("gatewayId")
	if gatewayID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway id is required"})
		return
	}
	adminID := actingAdminID(c)

	gw, err := h.gatewayRepo.GetByExternalID(c.Request.Context(), gatewayID)
	if err != nil {
		if errors.Is(err, storage.ErrGatewayNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "gateway not found"})
			return
		}
		h.logger.Error().Err(err).
			Str("gateway_id", gatewayID).
			Msg("failed to look up gateway for restart")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := h.syncHandler.reloadOne(c.Request.Context(), *gw); err != nil {
		h.logger.Error().Err(err).
			Str("gateway_id", gatewayID).
			Str("acting_admin_id", adminID).
			Msg("failed to restart gateway")
		c.JSON(http.StatusBadGateway, gin.H{"error": "gateway restart failed", "detail": err.Error()})
		return
	}

	h.logger.Info().
		Str("gateway_id", gatewayID).
		Str("acting_admin_id", adminID).
		Msg("gateway restarted by super-admin")

	c.JSON(http.StatusOK, gin.H{"message": "gateway restarted", "gateway_id": gatewayID})
}

// ─────────────────────────────────────────────────────────────────────────────
// Item 7 — Force-Decommission an Unresponsive Gateway
// ─────────────────────────────────────────────────────────────────────────────

// ForceDecommission handles
// POST /admin/v1/platform/gateways/:gatewayId/force-decommission
//
// Operator override — stops and removes the container and marks it
// decommissioned in the DB, bypassing ProjectScope/RBAC entirely.
// Used when a gateway is stuck and the normal tenant-facing path isn't available.
//
// Does NOT auto-recreate the container. A misbehaving container that needed
// force-removal could loop the same failure if auto-recreated.
// Re-provisioning is left as an explicit operator decision via the Provision flow.
func (h *PlatformHandler) ForceDecommission(c *gin.Context) {
	gatewayID := c.Param("gatewayId")
	if gatewayID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway id is required"})
		return
	}

	// Validate format: Must be the human-readable external_id
	if !strings.HasPrefix(gatewayID, "gw_") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid gateway ID: please provide the external_id (e.g. 'gw_xxxx') instead of the database UUID",
		})
		return
	}

	adminID := actingAdminID(c)

	if err := h.containerMgr.Decommission(c.Request.Context(), gatewayID); err != nil {
		h.logger.Error().Err(err).
			Str("gateway_id", gatewayID).
			Str("acting_admin_id", adminID).
			Msg("platform override: failed to stop container during force-decommission")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to stop container: %v", err),
		})
		return
	}

	if err := h.gatewayRepo.Decommission(c.Request.Context(), gatewayID); err != nil {
		if errors.Is(err, storage.ErrGatewayNotFound) {
			// Check if it is actually in the DB but already decommissioned (soft-deleted).
			status, statusErr := h.gatewayRepo.GetGatewayStatusPlatform(c.Request.Context(), gatewayID)
			if statusErr == nil && status == "decommissioned" {
				h.logger.Info().
					Str("gateway_id", gatewayID).
					Str("acting_admin_id", adminID).
					Msg("platform override: gateway force-decommissioned (DB row already decommissioned)")
				c.JSON(http.StatusOK, gin.H{
					"gateway_id": gatewayID,
					"status":     "decommissioned",
					"message":    "container removed; gateway is now OFFLINE — use the Provision flow to re-create if needed",
				})
				return
			}

			// If it doesn't exist in the DB at all, return 404.
			c.JSON(http.StatusNotFound, gin.H{"error": "gateway not found"})
			return
		}
		h.logger.Error().Err(err).
			Str("gateway_id", gatewayID).
			Str("acting_admin_id", adminID).
			Msg("platform override: failed to mark gateway decommissioned in DB")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Info().
		Str("gateway_id", gatewayID).
		Str("acting_admin_id", adminID).
		Msg("platform override: gateway force-decommissioned by super-admin")

	c.JSON(http.StatusOK, gin.H{
		"gateway_id": gatewayID,
		"status":     "decommissioned",
		"message":    "container removed; gateway is now OFFLINE — use the Provision flow to re-create if needed",
	})
}
