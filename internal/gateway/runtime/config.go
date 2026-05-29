package runtime

import "elitegate/internal/model"

type Snapshot struct {
	Routes []model.Route
}

func (s Snapshot) RouteCount() int {
	return len(s.Routes)
}
