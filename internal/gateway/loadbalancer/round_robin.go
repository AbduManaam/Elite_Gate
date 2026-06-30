package loadbalancer

import "sync/atomic"

type RoundRobin struct {
	counter atomic.Uint64
}

func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

func (r *RoundRobin) Name() string { return "round_robin" }

func (r *RoundRobin) Pick(pool []Target) (Target, error) {
	if len(pool) == 0 {
		return Target{}, ErrNoHealthyTargets
	}
	if len(pool) == 1 {
		return pool[0], nil
	}
	expanded := expandByWeight(pool)
	idx := r.counter.Add(1) % uint64(len(expanded))
	return expanded[idx], nil
}

func (r *RoundRobin) Release(_ Target) {}

//Convert server weights into multiple slots so that servers with higher weights are selected more often by the normal round-robin algorithm.
func expandByWeight(pool []Target) []Target {
	const maxWeight = 100

	allEqual := true
	for _, t := range pool {
		if t.Weight != pool[0].Weight {
			allEqual = false
			break
		}
	}
	if allEqual {
		return pool
	}

	expanded := make([]Target, 0, len(pool)*4)
	for _, t := range pool {
		w := t.Weight
		if w < 1 {
			w = 1
		}
		if w > maxWeight {
			w = maxWeight
		}
		for i := 0; i < w; i++ {
			expanded = append(expanded, t)
		}
	}
	return expanded
}
