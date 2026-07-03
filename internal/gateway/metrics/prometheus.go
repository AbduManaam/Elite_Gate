package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestCounter tracks the total number of HTTP requests processed by the gateway
	HTTPRequestCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_http_requests_total",
			Help: "Total number of HTTP requests processed by the gateway",
		},
		[]string{"path", "method", "status", "upstream"},
	)

	// HTTPRequestDuration tracks the latency of HTTP requests in seconds
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "gateway_http_request_duration_seconds",
			Help: "Latency of HTTP requests in seconds",
			// Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
			Buckets: []float64{1, 50, 100, 200, 300, 400, 500},
		},
		[]string{"path", "method", "status", "upstream"},
	)

	// ActiveRequests tracks the number of currently active in-flight requests
	ActiveRequests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_http_active_requests",
			Help: "Number of active in-flight requests",
		},
		[]string{"path", "method"},
	)
)
