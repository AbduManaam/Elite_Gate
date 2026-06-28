package loadbalancer

import "sync/atomic"

//global selection counter used to decide which server gets the next request.
//The counter only keeps track of how many picks have happened, not how many
//  requests are currently running on each server.
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
	// Selects the next target using a round-robin index over the weighted target list.
	expanded := expandByWeight(pool)

	// idx := counter.Add(1) % 4 [4 = total list of Target]
	//expanded[3]
	// Return =
	/*Target{
	ID: "B",
	URL: "http://server-b",
	}*/
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
