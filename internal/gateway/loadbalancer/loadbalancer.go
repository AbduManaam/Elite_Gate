package loadbalancer

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
