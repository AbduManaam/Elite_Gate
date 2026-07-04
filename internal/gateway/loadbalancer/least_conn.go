package loadbalancer

//Least Connection selects the backend server that currently has the fewest active (in-flight) requests.

type LeastConn struct {
	pickMu
	inFlight map[string]int64 //stores active request count per server.
} //Target.ID as the key eg. inFlight["server-a"] = 5  not=inFlight["http://10.0.0.1"] = 5

func NewLeastConn() *LeastConn {
	return &LeastConn{
		inFlight: make(map[string]int64),
	}
}
func (l *LeastConn) Name() string { return "least_conn" }

// Find the server with the fewest active requests, increase its count, and return that server.
func (l *LeastConn) Pick(pool []Target) (Target, error) {
	if len(pool) == 0 {
		return Target{}, ErrNoHealthyTargets
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	best := pool[0]
	bestCount := l.inFlight[best.ID]

	for _, t := range pool[1:] {
		c := l.inFlight[t.ID]
		if c < bestCount {
			best = t
			bestCount = c
		}
	}

	l.inFlight[best.ID]++
	return best, nil
}

func (l *LeastConn) Release(target Target) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if c, ok := l.inFlight[target.ID]; ok {
		if c <= 1 {
			delete(l.inFlight, target.ID)
		} else {
			l.inFlight[target.ID] = c - 1
		}
	}
}

// Returns a copy of current counts.
func (l *LeastConn) InFlight() map[string]int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make(map[string]int64, len(l.inFlight))
	for k, v := range l.inFlight {
		out[k] = v
	}
	return out
}
