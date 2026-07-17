package model

import "time"

// KPIValue wraps a scalar metric with its display unit.
type KPIValue struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// TimeSeriesPoint is a single (timestamp, value) pair for chart rendering.
type TimeSeriesPoint struct {
	Timestamp int64   `json:"timestamp"` // Unix milliseconds
	Value     float64 `json:"value"`
}

// MetricSeries is one labelled series of time-series points (e.g. one status code bucket).
type MetricSeries struct {
	Label  string            `json:"label"`
	Points []TimeSeriesPoint `json:"points"`
}

// DashboardSummary is the response payload for the project metrics dashboard.
// It aggregates the most common traffic KPIs so the frontend can load
// the entire overview panel with a single API call.
type DashboardSummary struct {
	ProjectID   string    `json:"project_id"`
	GeneratedAt time.Time `json:"generated_at"`

	// KPI cards
	RequestRate    KPIValue `json:"request_rate"`
	ErrorRate      KPIValue `json:"error_rate"`
	LatencyP50     KPIValue `json:"latency_p50"`
	LatencyP95     KPIValue `json:"latency_p95"`
	ActiveRequests KPIValue `json:"active_requests"`

	// KPI cards (extended)
	TotalRequests KPIValue `json:"total_requests"`
	LatencyAvg    KPIValue `json:"latency_avg"`
	ErrorRatePct  KPIValue `json:"error_rate_pct"`

	// Chart data
	RequestRateTrend        []TimeSeriesPoint      `json:"request_rate_trend"`
	LatencyAvgTrend         []TimeSeriesPoint      `json:"latency_avg_trend"`
	StatusBreakdown         []MetricSeries         `json:"status_breakdown"`
	TopRoutes               []MetricSeries         `json:"top_routes"`         // label = path, points = single latest value
	TopUpstreams            []MetricSeries         `json:"top_upstreams"`
	UpstreamHealth          []UpstreamHealthStatus `json:"upstream_health"`
	ActiveRequestsSparkline []TimeSeriesPoint      `json:"active_requests_sparkline"`
}

// UpstreamHealthStatus reports the live health of a single upstream target.
type UpstreamHealthStatus struct {
	Upstream string `json:"upstream"`
	Healthy  bool   `json:"healthy"`
}