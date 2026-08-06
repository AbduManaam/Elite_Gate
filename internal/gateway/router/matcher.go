package router

import (
	"net/http"
	"strings"

	"elitegate/internal/model"
)

func MatchHTTP(path string, method string, routes []model.Route) *model.Route {
	var best *model.Route
	bestScore := -1

	for i := range routes {
		rt := &routes[i]
		if rt.Protocol != "http" || !rt.Enabled {
			continue
		}

		if !matchMethod(rt.Methods, method) {
			continue
		}

		score := matchScore(rt, path)
		if score > bestScore {
			best = rt
			bestScore = score
		}
	}
	return best
}

func matchMethod(methods []string, reqMethod string) bool {
	if len(methods) == 0 || reqMethod == "" || reqMethod == http.MethodOptions {
		return true
	}
	for _, m := range methods {
		if strings.EqualFold(m, "ANY") || strings.EqualFold(m, "*") || strings.EqualFold(m, reqMethod) {
			return true
		}
	}
	return false
}

func matchScore(rt *model.Route, path string) int {
	if rt.Path == path {
		return 1000000 + len(rt.Path)
	}

	if strings.Contains(rt.Path, ":") {
		if matchParametric(rt.Path, path) {
			return 500000 + len(rt.Path)
		}
	}

	if rt.MatchType != "exact" && matchBoundaryPrefix(rt.Path, path) {
		return len(rt.Path)
	}

	return -1
}

func matchBoundaryPrefix(patternPath, reqPath string) bool {
	if !strings.HasPrefix(reqPath, patternPath) {
		return false
	}
	if strings.HasSuffix(patternPath, "/") {
		return true
	}
	if len(reqPath) == len(patternPath) {
		return true
	}
	return strings.HasPrefix(reqPath[len(patternPath):], "/")
}

func matchParametric(pattern, path string) bool {
	patternSegs := splitPath(pattern)
	pathSegs := splitPath(path)

	if len(patternSegs) != len(pathSegs) {
		return false
	}

	for i := 0; i < len(patternSegs); i++ {
		pSeg := patternSegs[i]
		rSeg := pathSegs[i]

		if strings.HasPrefix(pSeg, ":") {
			if rSeg == "" {
				return false
			}
		} else if pSeg != rSeg {
			return false
		}
	}
	return true
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return []string{}
	}
	return strings.Split(p, "/")
}
