package runtime

import (
	"elitegate/internal/gateway/loadbalancer"
	"elitegate/internal/model"
)

type Snapshot struct {
	Routes        []model.Route
	UpstreamPools map[string]UpstreamPool
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
	Targets  []loadbalancer.Target
	Strategy loadbalancer.Strategy
}

// No pointer,Bcz the method only reads data and does not modify anything.
// Snapshot is an in-memory cache of routes and load-balancer pools. The gateway reads from this cache instead of querying the database
// for every request.
func (s Snapshot) RouteCount() int {
	return len(s.Routes)
}
