package handler

import (
	"net/http"
	"sync"

	"elitegate/internal/gateway/proxy"
	"elitegate/internal/gateway/runtime"
	"elitegate/internal/model"
	"elitegate/internal/shared"
)

type DynamicProxy struct {
	loader *runtime.Loader
	hostMap map[string]string
	mu      sync.Mutex
	proxies map[string]*proxy.ReverseProxy
}

func NewDynamicProxy(loader *runtime.Loader, hostMap map[string]string) *DynamicProxy {
	return &DynamicProxy{
		loader:  loader,
		hostMap: hostMap,
		proxies: make(map[string]*proxy.ReverseProxy),
	}
}

func (d *DynamicProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt, ok := r.Context().Value(shared.ContextKeyRoute).(*model.Route)
	if !ok || rt == nil {
		http.Error(w, `{"error":"route not found"}`, http.StatusNotFound)
		return
	}

	p, err := d.getProxy(rt.UpstreamURL)
	if err != nil {
		http.Error(w, `{"error":"bad upstream"}`, http.StatusBadGateway)
		return
	}
	p.ServeHTTP(w, r)
}

func (d *DynamicProxy) getProxy(target string) (*proxy.ReverseProxy, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if p, ok := d.proxies[target]; ok {
		return p, nil
	}
	p, err := proxy.New(target, d.hostMap)
	if err != nil {
		return nil, err
	}
	d.proxies[target] = p
    return p, nil
}