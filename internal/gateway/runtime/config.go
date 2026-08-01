package runtime

import (
	"net"
	"strings"

	"elitegate/internal/gateway/loadbalancer"
	"elitegate/internal/model"
)

type DomainContext struct {
	Hostname      string
	Status        string
	RoutingStatus string
}

type Snapshot struct {
	Routes        []model.Route
	UpstreamPools map[string]UpstreamPool
	CustomDomains []model.CustomDomainSync
	DomainMap     map[string]DomainContext
}

// NormalizeHost cleans and normalizes a host string by trimming whitespace,
// converting to lowercase, removing trailing dots (before and after port stripping),
// and stripping any port if present.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.ToLower(host)

	// Remove trailing dot after port, e.g. example.com:443.
	host = strings.TrimSuffix(host, ".")

	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Remove trailing dot if hostname itself ended with dot, e.g. example.com.
	host = strings.TrimSuffix(host, ".")
	return host
}

// LookupDomain performs an O(1) normalized lookup for custom domains in Gateway memory.
func (s Snapshot) LookupDomain(host string) (DomainContext, bool) {
	if len(s.DomainMap) == 0 {
		return DomainContext{}, false
	}
	norm := NormalizeHost(host)
	ctx, ok := s.DomainMap[norm]
	return ctx, ok
}

//Stores all backend servers for each upstream.
// Order Service (upstream-1)
// ├── order-service-pod-1
// ├── order-service-pod-2
// └── order-service-pod-3

// User Service (upstream-2)
// ├── user-service-pod-1
// └── user-service-pod-2
type UpstreamPool struct {
	ProjectID  string
	Targets    []loadbalancer.Target
	Strategy   loadbalancer.Strategy
	HealthPath string // health probe path for all targets in this pool (e.g. "/health")
}

// No pointer,Bcz the method only reads data and does not modify anything.
// Snapshot is an in-memory cache of routes and load-balancer pools. The gateway reads from this cache instead of querying the database
// for every request.
func (s Snapshot) RouteCount() int {
	return len(s.Routes)
}
