package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

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
		log.Error().Err(err).Msg("failed to build reload request")
		return fmt.Errorf("gateway %s: failed to build request: %w", g.ID, err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("reload request failed")
		return fmt.Errorf("gateway %s: request failed: %w", g.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn().Int("status", resp.StatusCode).Msg("reload returned non-200")
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
		h.logger.Error().Err(err).Msg("failed to list active gateways for reload")
		// Do NOT leak the raw DB error to the client.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve active gateways"})
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

// reloadURL builds the reload endpoint URL for a gateway.
// 0.0.0.0 is treated as localhost (Docker / local dev convenience).
func reloadURL(g storage.GatewayRecord) string {
	host := g.EndpointIP
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%s/reload", host, g.Port)
}
