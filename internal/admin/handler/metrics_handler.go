package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"elitegate/internal/admin/service"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// MetricsHandler is intentionally thin: parse HTTP params, call the
// service, write the response. All PromQL, caching, and normalization
// lives in service.MetricsService.
type MetricsHandler struct {
	svc         *service.MetricsService
	gatewayRepo *storage.GatewayRepo
	logger      zerolog.Logger
}

func NewMetricsHandler(svc *service.MetricsService, gatewayRepo *storage.GatewayRepo, logger zerolog.Logger) *MetricsHandler {
	return &MetricsHandler{
		svc:         svc,
		gatewayRepo: gatewayRepo,
		logger:      logger,
	}
}

// QueryRange handles GET /admin/v1/projects/:projectId/metrics/query-range?metric=request_rate&range=1h&step=60s
func (h *MetricsHandler) QueryRange(c *gin.Context) {
	projectID := c.Param("projectId")
	metricName := c.Query("metric")

	if !service.IsValidMetricName(metricName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown metric name"})
		return
	}

	window, err := parseWindow(c.DefaultQuery("range", "1h"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	step := c.DefaultQuery("step", "60s")

	points, err := h.svc.QueryRange(c.Request.Context(), projectID, service.MetricName(metricName), window, step)
	if err != nil {
		h.logger.Error().Err(err).Str("project_id", projectID).Str("metric", metricName).Msg("metrics query-range failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": "metrics backend unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"points": points})
}

// QueryInstant handles GET /admin/v1/projects/:projectId/metrics/query?metric=active_requests
func (h *MetricsHandler) QueryInstant(c *gin.Context) {
	projectID := c.Param("projectId")
	metricName := c.Query("metric")

	if !service.IsValidMetricName(metricName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown metric name"})
		return
	}

	val, err := h.svc.QueryInstant(c.Request.Context(), projectID, service.MetricName(metricName))
	if err != nil {
		h.logger.Error().Err(err).Str("project_id", projectID).Str("metric", metricName).Msg("metrics query failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": "metrics backend unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"value": val})
}

// Summary handles GET /admin/v1/projects/:projectId/metrics/summary
// Returns every KPI + the main chart + status breakdown in one response,
// for the observability overview page.
func (h *MetricsHandler) Summary(c *gin.Context) {
	projectID := c.Param("projectId")

	summary, err := h.svc.DashboardSummary(c.Request.Context(), projectID)
	if err != nil {
		h.logger.Error().Err(err).Str("project_id", projectID).Msg("metrics summary failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": "metrics backend unavailable"})
		return
	}

	c.JSON(http.StatusOK, summary)
}

func (h *MetricsHandler) resolveServiceName(c *gin.Context, serviceName string) string {
	projectID := c.Param("projectId")
	if projectID != "" && serviceName == "elitegate-gateway" && h.gatewayRepo != nil {
		gateways, _, err := h.gatewayRepo.ListByProject(c.Request.Context(), projectID, 10, 0)
		if err == nil {
			for _, gw := range gateways {
				if gw.Plan == "dedicated" && gw.Status == "active" {
					return fmt.Sprintf("elitegate-gateway-%s", gw.ExternalID)
				}
			}
		}
	}
	return serviceName
}

// SystemMetrics handles GET /admin/v1/platform/metrics/system?service=elitegate-gateway
func (h *MetricsHandler) SystemMetrics(c *gin.Context) {
	serviceName := h.resolveServiceName(c, c.DefaultQuery("service", "elitegate-gateway"))

	cpu, err := h.svc.QuerySystemInstant(c.Request.Context(), serviceName, "cpu")
	if err != nil {
		h.logger.Error().Err(err).Str("service", serviceName).Str("metric", "cpu").Msg("system metrics query failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": "metrics backend unavailable"})
		return
	}

	mem, err := h.svc.QuerySystemInstant(c.Request.Context(), serviceName, "memory")
	if err != nil {
		h.logger.Error().Err(err).Str("service", serviceName).Str("metric", "memory").Msg("system metrics query failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": "metrics backend unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"cpu_percent":  cpu,
		"memory_bytes": mem,
	})
}

// SystemMetricsRange handles GET /admin/v1/platform/metrics/system/range?service=elitegate-gateway&metric=cpu&range=1h&step=60s
func (h *MetricsHandler) SystemMetricsRange(c *gin.Context) {
	serviceName := h.resolveServiceName(c, c.DefaultQuery("service", "elitegate-gateway"))
	metric := c.Query("metric") // "cpu" | "memory"

	window, err := parseWindow(c.DefaultQuery("range", "1h"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	step := c.DefaultQuery("step", "60s")

	points, err := h.svc.QuerySystemRange(c.Request.Context(), serviceName, metric, window, step)
	if err != nil {
		h.logger.Error().Err(err).Str("service", serviceName).Str("metric", metric).Msg("system metrics range failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": "metrics backend unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"points": points})
}

var errWindowTooLarge = errors.New("range too large, max 7d")

func parseWindow(raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid range: %s", raw)
	}
	if d > 7*24*time.Hour {
		return 0, errWindowTooLarge
	}
	return d, nil
}
