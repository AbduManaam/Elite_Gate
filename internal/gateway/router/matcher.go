package router

import (
	"strings"

	"elitegate/internal/model"
)

func MatchHTTP(path string, routes []model.Route) *model.Route {
	var best *model.Route
	bestLen := -1

	for i := range routes {
		rt := &routes[i]
		if rt.Protocol != "http" || !rt.Enabled {
			continue
		}
		switch rt.MatchType {
		case "exact":
			if path == rt.Path && len(rt.Path) > bestLen {
				best, bestLen = rt, len(rt.Path)
			}
		default: // prefix
			if strings.HasPrefix(path, rt.Path) && len(rt.Path) > bestLen {
				best, bestLen = rt, len(rt.Path)
			}
		}
	}
	return best
}

// http://localhost:8080/api/user
