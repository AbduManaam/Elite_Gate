package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"elitegate/internal/model"
	"elitegate/internal/promclient"
)

// MetricsService owns all PromQL query construction and response
// normalization. Handlers never see raw Prometheus responses or write
// PromQL themselves — this is the only place both happen.
type MetricsService struct {
	client *promclient.Client
	cache  *metricsCache
}

func NewMetricsService(client *promclient.Client, cacheTTL time.Duration) *MetricsService {
	return &MetricsService{
		client: client,
		cache:  newMetricsCache(cacheTTL),
	}
}

// MetricName is the fixed, whitelisted set of queries the frontend can ask
// for. The client never sends raw PromQL — this is what stops one tenant's
// viewer from reading another tenant's metrics or querying arbitrary series.
// MetricName defines the fixed set of metrics the frontend can request.
type MetricName string

const (
	MetricRequestRate     MetricName = "request_rate"
	MetricTotalRequests   MetricName = "total_requests"
	MetricLatencyAvg      MetricName = "latency_avg"
	MetricLatencyP50      MetricName = "latency_p50"
	MetricLatencyP95      MetricName = "latency_p95"
	MetricErrorRatePct    MetricName = "error_rate_pct"
	MetricActiveRequests  MetricName = "active_requests"
	MetricStatusBreakdown MetricName = "status_breakdown"
	MetricTopRoutes       MetricName = "top_routes"
	MetricTopUpstreams    MetricName = "top_upstreams"
	MetricUpstreamHealth  MetricName = "upstream_health"
)

var ErrUnknownMetric = fmt.Errorf("unknown metric name")

var queryBuilders = map[MetricName]func(projectID string) string{
	MetricRequestRate: func(p string) string {
		return fmt.Sprintf(`sum(rate(gateway_http_requests_total{project_id=%q}[5m]))`, p)
	},
	MetricTotalRequests: func(p string) string {
		return fmt.Sprintf(`sum(increase(gateway_http_requests_total{project_id=%q}[5m]))`, p)
	},
	MetricLatencyAvg: func(p string) string {
		return fmt.Sprintf(
			`(sum(rate(gateway_http_request_duration_seconds_sum{project_id=%q}[5m])) / sum(rate(gateway_http_request_duration_seconds_count{project_id=%q}[5m]))) * 1000`,
			p, p,
		)
	},
	MetricLatencyP50: func(p string) string {
		return fmt.Sprintf(`histogram_quantile(0.50, sum(rate(gateway_http_request_duration_seconds_bucket{project_id=%q}[5m])) by (le)) * 1000`, p)
	},
	MetricLatencyP95: func(p string) string {
		return fmt.Sprintf(`histogram_quantile(0.95, sum(rate(gateway_http_request_duration_seconds_bucket{project_id=%q}[5m])) by (le)) * 1000`, p)
	},
	MetricErrorRatePct: func(p string) string {
		return fmt.Sprintf(
			`(sum(rate(gateway_http_requests_total{project_id=%q, status=~"5.."}[5m])) / sum(rate(gateway_http_requests_total{project_id=%q}[5m]))) * 100`,
			p, p,
		)
	},
	MetricActiveRequests: func(p string) string {
		return fmt.Sprintf(`sum(gateway_http_active_requests{project_id=%q})`, p)
	},
	MetricStatusBreakdown: func(p string) string {
		return fmt.Sprintf(`sum(rate(gateway_http_requests_total{project_id=%q}[5m])) by (status)`, p)
	},
	MetricTopRoutes: func(p string) string {
		return fmt.Sprintf(`topk(5, sum(rate(gateway_http_requests_total{project_id=%q}[5m])) by (path))`, p)
	},
	MetricTopUpstreams: func(p string) string {
		return fmt.Sprintf(`topk(5, sum(rate(gateway_http_requests_total{project_id=%q}[5m])) by (upstream))`, p)
	},
	MetricUpstreamHealth: func(p string) string {
		return fmt.Sprintf(`gateway_upstream_health_status{project_id=%q}`, p) // instant vector; 1=healthy, 0=unhealthy, per upstream
	},
}

var systemQueryBuilders = map[string]func(job string) string{
	"cpu":    func(job string) string { return fmt.Sprintf(`rate(process_cpu_seconds_total{job=%q}[5m]) * 100`, job) },
	"memory": func(job string) string { return fmt.Sprintf(`process_resident_memory_bytes{job=%q}`, job) },
}

func IsValidMetricName(name string) bool {
	_, ok := queryBuilders[MetricName(name)]
	return ok
}

func sanitizeFloat(val float64) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0.0
	}
	return val
}

// QueryRange returns a chart-ready series for one metric, cached for the
// configured TTL per (project, metric, range, step).
func (s *MetricsService) QueryRange(ctx context.Context, projectID string, metric MetricName, window time.Duration, step string) ([]model.TimeSeriesPoint, error) {
	tmpl, ok := queryBuilders[metric]
	if !ok {
		return nil, ErrUnknownMetric
	}

	cacheKey := fmt.Sprintf("range:%s:%s:%s:%s", projectID, metric, window, step)
	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.([]model.TimeSeriesPoint), nil
	}

	promql := tmpl(projectID)
	now := time.Now()

	series, err := s.client.QueryRange(ctx, promql, now.Add(-window), now, step)
	if err != nil {
		return nil, fmt.Errorf("query range for %s: %w", metric, err)
	}

	points := flattenSingleSeries(series)
	s.cache.set(cacheKey, points)
	return points, nil
}

// QueryRangeGrouped is like QueryRange but keeps each label group as its own
// named series — used for status_breakdown, where each status code is its
// own line rather than being summed together.
func (s *MetricsService) QueryRangeGrouped(ctx context.Context, projectID string, metric MetricName, window time.Duration, step string, groupLabel string) ([]model.MetricSeries, error) {
	tmpl, ok := queryBuilders[metric]
	if !ok {
		return nil, ErrUnknownMetric
	}

	cacheKey := fmt.Sprintf("range-grouped:%s:%s:%s:%s", projectID, metric, window, step)
	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.([]model.MetricSeries), nil
	}

	promql := tmpl(projectID)
	now := time.Now()

	raw, err := s.client.QueryRange(ctx, promql, now.Add(-window), now, step)
	if err != nil {
		return nil, fmt.Errorf("query range grouped for %s: %w", metric, err)
	}

	out := make([]model.MetricSeries, 0, len(raw))
	for _, series := range raw {
		out = append(out, model.MetricSeries{
			Label:  series.Labels[groupLabel],
			Points: toPoints(series.Samples),
		})
	}

	s.cache.set(cacheKey, out)
	return out, nil
}

// QueryInstant returns a single current value — used for KPI cards.
func (s *MetricsService) QueryInstant(ctx context.Context, projectID string, metric MetricName) (float64, error) {
	tmpl, ok := queryBuilders[metric]
	if !ok {
		return 0, ErrUnknownMetric
	}

	cacheKey := fmt.Sprintf("instant:%s:%s", projectID, metric)
	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(float64), nil
	}

	promql := tmpl(projectID)
	samples, err := s.client.Query(ctx, promql)
	if err != nil {
		return 0, fmt.Errorf("query instant for %s: %w", metric, err)
	}

	var val float64
	if len(samples) > 0 {
		val = sanitizeFloat(samples[0].Value)
	}

	s.cache.set(cacheKey, val)
	return val, nil
}

// QuerySystemInstant returns a single current value for a system-level metric (cpu/memory) for a specific job.
func (s *MetricsService) QuerySystemInstant(ctx context.Context, job string, metric string) (float64, error) {
	builder, ok := systemQueryBuilders[metric]
	if !ok {
		return 0, fmt.Errorf("unknown system metric name: %s", metric)
	}

	cacheKey := fmt.Sprintf("system-instant:%s:%s", job, metric)
	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(float64), nil
	}

	promql := builder(job)
	samples, err := s.client.Query(ctx, promql)
	if err != nil {
		return 0, fmt.Errorf("query system instant for %s: %w", metric, err)
	}

	var val float64
	if len(samples) > 0 {
		val = sanitizeFloat(samples[0].Value)
	}

	s.cache.set(cacheKey, val)
	return val, nil
}

func (s *MetricsService) QuerySystemRange(ctx context.Context, job, metric string, window time.Duration, step string) ([]model.TimeSeriesPoint, error) {
	builder, ok := systemQueryBuilders[metric]
	if !ok {
		return nil, fmt.Errorf("unknown system metric name: %s", metric)
	}

	cacheKey := fmt.Sprintf("system-range:%s:%s:%s:%s", job, metric, window, step)
	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.([]model.TimeSeriesPoint), nil
	}

	promql := builder(job)
	now := time.Now()
	series, err := s.client.QueryRange(ctx, promql, now.Add(-window), now, step)
	if err != nil {
		return nil, fmt.Errorf("query system range for %s: %w", metric, err)
	}

	points := flattenSingleSeries(series)
	s.cache.set(cacheKey, points)
	return points, nil
}

// QueryInstantGrouped runs an instant query that returns multiple labeled
// results (e.g. topk by path/upstream, or a health-status vector) and keeps
// each label as its own MetricSeries with a single current-value point.
func (s *MetricsService) QueryInstantGrouped(ctx context.Context, projectID string, metric MetricName, groupLabel string) ([]model.MetricSeries, error) {
	tmpl, ok := queryBuilders[metric]
	if !ok {
		return nil, ErrUnknownMetric
	}

	cacheKey := fmt.Sprintf("instant-grouped:%s:%s", projectID, metric)
	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.([]model.MetricSeries), nil
	}

	promql := tmpl(projectID)
	samples, err := s.client.Query(ctx, promql)
	if err != nil {
		return nil, fmt.Errorf("query instant grouped for %s: %w", metric, err)
	}

	out := make([]model.MetricSeries, 0, len(samples))
	for _, sample := range samples {
		out = append(out, model.MetricSeries{
			Label:  sample.Labels[groupLabel],
			Points: []model.TimeSeriesPoint{{Timestamp: sample.Timestamp.UnixMilli(), Value: sanitizeFloat(sample.Value)}},
		})
	}

	s.cache.set(cacheKey, out)
	return out, nil
}

// DashboardSummary aggregates every KPI + the main trend chart + status
// breakdown into a single response, for the observability overview page.
// This is one cache-checked call per underlying metric internally, but one
// HTTP round trip for the frontend.
func (s *MetricsService) DashboardSummary(ctx context.Context, projectID string) (*model.DashboardSummary, error) {
	cacheKey := "summary:" + projectID
	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(*model.DashboardSummary), nil
	}

	requestRate, err := s.QueryInstant(ctx, projectID, MetricRequestRate)
	if err != nil {
		return nil, err
	}
	errorRatePct, err := s.QueryInstant(ctx, projectID, MetricErrorRatePct)
	if err != nil {
		return nil, err
	}
	p50, err := s.QueryInstant(ctx, projectID, MetricLatencyP50)
	if err != nil {
		return nil, err
	}
	p95, err := s.QueryInstant(ctx, projectID, MetricLatencyP95)
	if err != nil {
		return nil, err
	}
	active, err := s.QueryInstant(ctx, projectID, MetricActiveRequests)
	if err != nil {
		return nil, err
	}
	totalRequests, err := s.QueryInstant(ctx, projectID, MetricTotalRequests)
	if err != nil {
		return nil, err
	}
	latencyAvg, err := s.QueryInstant(ctx, projectID, MetricLatencyAvg)
	if err != nil {
		return nil, err
	}

	trend, err := s.QueryRange(ctx, projectID, MetricRequestRate, time.Hour, "60s")
	if err != nil {
		return nil, err
	}
	statusBreakdown, err := s.QueryRangeGrouped(ctx, projectID, MetricStatusBreakdown, time.Hour, "60s", "status")
	if err != nil {
		return nil, err
	}
	activeSparkline, err := s.QueryRange(ctx, projectID, MetricActiveRequests, 15*time.Minute, "15s")
	if err != nil {
		return nil, err
	}
	topRoutes, err := s.QueryInstantGrouped(ctx, projectID, MetricTopRoutes, "path")
	if err != nil {
		return nil, err
	}
	topUpstreams, err := s.QueryInstantGrouped(ctx, projectID, MetricTopUpstreams, "upstream")
	if err != nil {
		return nil, err
	}
	healthSeries, err := s.QueryInstantGrouped(ctx, projectID, MetricUpstreamHealth, "upstream")
	if err != nil {
		return nil, err
	}

	upstreamHealth := make([]model.UpstreamHealthStatus, 0, len(healthSeries))
	for _, series := range healthSeries {
		healthy := len(series.Points) > 0 && series.Points[0].Value == 1
		upstreamHealth = append(upstreamHealth, model.UpstreamHealthStatus{Upstream: series.Label, Healthy: healthy})
	}

	summary := &model.DashboardSummary{
		ProjectID:               projectID,
		GeneratedAt:             time.Now(),
		RequestRate:             model.KPIValue{Value: requestRate, Unit: "req/s"},
		ErrorRate:               model.KPIValue{Value: errorRatePct, Unit: "%"},
		ErrorRatePct:            model.KPIValue{Value: errorRatePct, Unit: "%"},
		LatencyP50:              model.KPIValue{Value: p50, Unit: "ms"},
		LatencyP95:              model.KPIValue{Value: p95, Unit: "ms"},
		ActiveRequests:          model.KPIValue{Value: active, Unit: "count"},
		TotalRequests:           model.KPIValue{Value: totalRequests, Unit: "count"},
		LatencyAvg:              model.KPIValue{Value: latencyAvg, Unit: "ms"},
		RequestRateTrend:        trend,
		StatusBreakdown:         statusBreakdown,
		TopRoutes:               topRoutes,
		TopUpstreams:            topUpstreams,
		UpstreamHealth:          upstreamHealth,
		ActiveRequestsSparkline: activeSparkline,
	}

	s.cache.set(cacheKey, summary)
	return summary, nil
}

func flattenSingleSeries(series []promclient.Series) []model.TimeSeriesPoint {
	if len(series) == 0 {
		return []model.TimeSeriesPoint{}
	}
	return toPoints(series[0].Samples)
}

func toPoints(samples []promclient.Sample) []model.TimeSeriesPoint {
	points := make([]model.TimeSeriesPoint, 0, len(samples))
	for _, s := range samples {
		points = append(points, model.TimeSeriesPoint{
			Timestamp: s.Timestamp.UnixMilli(),
			Value:     sanitizeFloat(s.Value),
		})
	}
	return points
}
