package promclient

import "time"

// Prometheus client package.
// Sends PromQL queries, converts Prometheus JSON responses
// into simple Go structs, communication with Prometheus.
// Does not know anything about application's business logic.

type Sample struct {
	Timestamp time.Time
	Value     float64
}

type Series struct {
	Labels  map[string]string
	Samples []Sample
}

type InstantSample struct {
	Labels    map[string]string
	Timestamp time.Time
	Value     float64
}

// RangePoint and RangeSeries mirror Series/Sample but use the naming
// convention used by the range query path in client.go.
type RangePoint = Sample
type RangeSeries = Series
