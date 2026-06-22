package loadbalancer

// This file defines the core of the load balancer.it acts as a blueprint for deciding which
// backend server should handle each incoming request.:

// Target – Represents a backend server.
// Strategy – Defines how a server is selected (Round Robin, Least Connection, etc.).
// NewStrategy – Creates the correct load-balancing strategy based on configuration.
// pickMu – Ensures shared data is accessed safely when multiple requests are handled at the same time.

import (
	"errors"
	"sync"
)

var ErrNoHealthyTargets = errors.New("Loadbalancer: no Healthy targets available ")

type Target struct {
	ID     string
	URL    string
	Weight int
}
type Strategy interface {
	Pick(pool []Target) (Target, error)
	Release(target Target)
	Name() string
}

func NewStrategy(name string) Strategy {
	switch name {
	case "least_conn":
		return NewLeastConn()
	case "round_robin", "":
		return NewRoundRobin()
	default:
		return NewRoundRobin()
	}
}

// pickMu is a helper that prevents multiple goroutines from updating the
// same counters at the same time, keeping the data correct and safe.
type pickMu struct {
	mu sync.RWMutex
}
