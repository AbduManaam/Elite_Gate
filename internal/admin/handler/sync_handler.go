package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"elitegate/helper"
	"elitegate/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// reloadOne sends a single POST /reload to one gateway and returns an
// error describing what went wrong, or nil on success. This is the
// single-target unit both the bulk Reload handler and the platform-level
// single-gateway restart endpoint call — logic lives in exactly one place.
func (h *SyncHandler) reloadOne(ctx context.Context, g storage.GatewayRecord) error {
	url := reloadURL(g)
	log := h.logger.With().Str("gateway_id", g.ID).Str("url", url).Logger()

	reqCtx, cancel := context.WithTimeout(ctx, gatewayReloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("gateway %s: failed to build request: %w", g.ID, err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gateway %s: request failed: %w", g.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway %s: unexpected status %d", g.ID, resp.StatusCode)
	}

	log.Info().Msg("gateway reloaded successfully")
	return nil
}

const gatewayReloadTimeout = 5 * time.Second

type SyncHandler struct {
	gatewayRepo *storage.GatewayRepo
	logger      zerolog.Logger
}

func NewSyncHandler(gatewayRepo *storage.GatewayRepo, logger zerolog.Logger) *SyncHandler {
	return &SyncHandler{
		gatewayRepo: gatewayRepo,
		logger:      logger.With().Str("handler", "sync").Logger(),
	}
}

// Reload fans out a POST /reload to every active gateway node concurrently.
// Returns 207 Multi-Status when at least one node fails, 200 when all succeed.
func (h *SyncHandler) Reload(c *gin.Context) {
	gateways, err := h.gatewayRepo.ListActive(c.Request.Context())
	if err != nil {
		helper.RespondInternalError(c, h.logger, err, "failed to retrieve active gateways")
		return
	}

	if len(gateways) == 0 {
		h.logger.Info().Msg("no active gateways found; reload is a no-op")
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "no active gateways to reload",
		})
		return
	}

	h.logger.Info().Int("gateway_count", len(gateways)).Msg("fanning out reload to active gateways")

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []string
	)

	for _, gw := range gateways {
		wg.Add(1)
		go func(g storage.GatewayRecord) {
			defer wg.Done()
			if err := h.reloadOne(c.Request.Context(), g); err != nil {
				mu.Lock()
				errs = append(errs, err.Error())
				mu.Unlock()
			}
		}(gw)
	}

	wg.Wait()

	if len(errs) > 0 {
		h.logger.Warn().
			Strs("errors", errs).
			Int("failed", len(errs)).
			Int("total", len(gateways)).
			Msg("partial reload: some gateways failed")

		c.JSON(http.StatusMultiStatus, gin.H{
			"status":  "partial_success",
			"errors":  errs,
			"message": "reload triggered, but some nodes failed to sync",
		})
		return
	}

	h.logger.Info().Int("gateway_count", len(gateways)).Msg("all gateways reloaded successfully")
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "all gateway caches reloaded successfully",
	})
}

// ReloadProject targets and reloads only the active gateways belonging to the specific project.
func (h *SyncHandler) ReloadProject(c *gin.Context) {
	tcVal, exists := c.Get("tenant_ctx")
	if !exists {
		helper.RespondInternalError(c, h.logger, nil, "tenant context missing")
		return
	}
	tc := tcVal.(storage.TenantContext)

	gateways, _, err := h.gatewayRepo.ListByProject(c.Request.Context(), tc.ProjectID.String(), 0, 0)
	if err != nil {
		helper.RespondInternalError(c, h.logger.With().Str("project_id", tc.ProjectID.String()).Logger(), err, "failed to retrieve project gateways")
		return
	}

	var activeGateways []storage.GatewayRecord
	for _, gw := range gateways {
		if gw.Status == "active" {
			activeGateways = append(activeGateways, gw)
		}
	}

	if len(activeGateways) == 0 {
		h.logger.Info().Str("project_id", tc.ProjectID.String()).Msg("no active gateways found for project; reload is a no-op")
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "no active gateways to reload for this project",
		})
		return
	}

	h.logger.Info().Str("project_id", tc.ProjectID.String()).Int("gateway_count", len(activeGateways)).Msg("fanning out reload to project active gateways")

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []string
	)

	for _, gw := range activeGateways {
		wg.Add(1)
		go func(g storage.GatewayRecord) {
			defer wg.Done()
			if err := h.reloadOne(c.Request.Context(), g); err != nil {
				mu.Lock()
				errs = append(errs, err.Error())
				mu.Unlock()
			}
		}(gw)
	}

	wg.Wait()

	if len(errs) > 0 {
		h.logger.Warn().
			Strs("errors", errs).
			Int("failed", len(errs)).
			Int("total", len(activeGateways)).
			Str("project_id", tc.ProjectID.String()).
			Msg("partial reload: some project gateways failed")

		c.JSON(http.StatusMultiStatus, gin.H{
			"status":  "partial_success",
			"errors":  errs,
			"message": "reload triggered, but some project nodes failed to sync",
		})
		return
	}

	h.logger.Info().Str("project_id", tc.ProjectID.String()).Int("gateway_count", len(activeGateways)).Msg("all project gateways reloaded successfully")
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "all project gateway caches reloaded successfully",
	})
}

// reloadURL builds the reload endpoint URL for a gateway.
// 0.0.0.0 is treated as localhost (Docker / local dev convenience).
func reloadURL(g storage.GatewayRecord) string {
	host := g.EndpointIP
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%s/reload", host, g.Port)
}
