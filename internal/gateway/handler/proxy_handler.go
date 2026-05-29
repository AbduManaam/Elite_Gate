package handler

import (
	"elitegate/internal/gateway/proxy"
	"elitegate/internal/gateway/router"
	"elitegate/internal/gateway/runtime"
	"net/http"
	"strings"
	"sync"
)

// Client Request
//       ↓
// Get routes from memory
//       ↓
// Find matching route
//       ↓
// No route? → 404
//       ↓
// Create/Get reverse proxy
//       ↓
// Proxy failed? → 502
//       ↓
// Forward request to backend
//       ↓
// Return backend response to client

type DynamicProxy struct {
	loader  *runtime.Loader
	mu      sync.Mutex
	proxies map[string]*proxy.ReverseProxy
}

func NewDynamicProxy(loader *runtime.Loader) *DynamicProxy {
	return &DynamicProxy{
		loader:  loader,
		proxies: make(map[string]*proxy.ReverseProxy),
	}
}

func (d *DynamicProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	snap := d.loader.Current()

	rt := router.MatchHTTP(r.URL.Path, snap.Routes)
	if rt == nil {
		http.Error(w, `{"error":"route not found"}`, http.StatusBadRequest)
		return
	}
	p, err := d.getProxy(rt.UpstreamURL)
	if err != nil {
		http.Error(w, `{"error":"bad upstream"}`, http.StatusBadGateway)
		return
	}

	// Strip "/api" prefix before forwarding
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	p.ServeHTTP(w, r)
}

func (d *DynamicProxy) getProxy(target string) (*proxy.ReverseProxy, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if p, ok := d.proxies[target]; ok {
		return p, nil
	}
	p, err := proxy.New(target)
	if err != nil {
		return nil, err
	}
	d.proxies[target] = p
	return p, nil
}
