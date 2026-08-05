package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"elitegate/internal/shared"
)

// ReverseProxy wraps httputil.ReverseProxy with production config
type ReverseProxy struct {
	proxy *httputil.ReverseProxy
}

// New creates a new ReverseProxy targeting the given upstream URL
func New(targetURL string, hostMap map[string]string) (*ReverseProxy, error) {
	targetURL = RewriteTargetURL(targetURL, hostMap)
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
		originalHost := req.Host
		originalDirector(req)

		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Host", originalHost)
		req.Header.Set("X-Gateway", "elitegate/1.0")

		// Forward authenticated identity as trusted headers so upstreams
		// can use them directly without re-validating credentials.
		if clientID, ok := req.Context().Value(shared.ContextKeyClientID).(string); ok && clientID != "" {
			req.Header.Set("X-Client-ID", clientID)
		}
		if role, ok := req.Context().Value(shared.ContextKeyRole).(string); ok && role != "" {
			req.Header.Set("X-Client-Role", role)
		}
		if scopes, ok := req.Context().Value(shared.ContextKeyScopes).([]string); ok && len(scopes) > 0 {
			req.Header.Set("X-Client-Scopes", strings.Join(scopes, ","))
		}

		// Strip raw API key — upstreams use X-Client-* headers above.
		req.Header.Del("X-API-Key")
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

// RewriteTargetURL applies an optional hostname mapping for local development.
func RewriteTargetURL(raw string, hostMap map[string]string) string {
	if len(hostMap) == 0 {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	if u.Scheme == "" && u.Host == "" && strings.Contains(raw, ":") {
		u, err = url.Parse("tcp://" + raw)
		if err != nil {
			return raw
		}
	}

	host := u.Hostname()
	if host == "" {
		return raw
	}

	mapped, ok := hostMap[host]
	if !ok {
		return raw
	}

	if strings.Contains(mapped, ":") {
		u.Host = mapped
	} else if u.Port() != "" {
		u.Host = net.JoinHostPort(mapped, u.Port())
	} else {
		u.Host = mapped
	}

	if u.Scheme == "tcp" && !strings.Contains(raw, "://") {
		return u.Host
	}
	return u.String()
}

// ServeHTTP implements http.Handler
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rp.proxy.ServeHTTP(w, r)
}
