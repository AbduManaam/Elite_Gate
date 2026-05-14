package proxy

import (
    "fmt"
    "net/http"
    "net/http/httputil"
    "net/url"
    "time"
)

// ReverseProxy wraps httputil.ReverseProxy with production config
type ReverseProxy struct {
    proxy *httputil.ReverseProxy
}

// New creates a new ReverseProxy targeting the given upstream URL
func New(targetURL string) (*ReverseProxy, error) {
    target, err := url.Parse(targetURL)
    if err != nil {
        return nil, fmt.Errorf("proxy.New: invalid target URL %q: %w", targetURL, err)
    }

    // Custom transport with production-grade timeouts
    transport := &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 20,
        IdleConnTimeout:     90 * time.Second,
        DisableCompression:  true, // Proxy streams as-is
    }

    p := httputil.NewSingleHostReverseProxy(target)
    p.Transport = transport

    // Custom director — runs before the request is forwarded
    originalDirector := p.Director
    p.Director = func(req *http.Request) {
        originalDirector(req)
        req.Header.Set("X-Forwarded-Host", req.Host)
        req.Header.Set("X-Gateway", "edgecore/1.0")
        req.Host = target.Host
    }

    // Custom error handler — prevents panic on upstream failure
    p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
        if r.Context().Err() != nil {
            w.WriteHeader(http.StatusRequestTimeout)
            return
        }
        http.Error(w,
            fmt.Sprintf(`{"error":"upstream unavailable","detail":"%v"}`, err),
            http.StatusBadGateway,
        )
    }

    return &ReverseProxy{proxy: p}, nil
}

// ServeHTTP implements http.Handler
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    rp.proxy.ServeHTTP(w, r)
}