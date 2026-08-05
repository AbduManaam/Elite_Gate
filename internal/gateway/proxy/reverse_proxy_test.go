package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"elitegate/internal/gateway/proxy"
)

func TestReverseProxy_UpstreamForwarding(t *testing.T) {
	var (
		receivedHost          string
		receivedForwardedHost string
		receivedMethod        string
		receivedPath          string
		receivedQuery         string
		receivedAuth          string
		receivedContentType   string
		receivedBody          string
	)

	// Upstream httptest server simulating Cloudflare Tunnel or external HTTPS upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		receivedForwardedHost = r.Header.Get("X-Forwarded-Host")
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		receivedAuth = r.Header.Get("Authorization")
		receivedContentType = r.Header.Get("Content-Type")

		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)

		// Reject request if Host header remains client host instead of target host
		targetURL, _ := url.Parse(r.URL.String())
		_ = targetURL
		if r.Host == "gw-da443dc4.elitegateway.site" {
			http.Error(w, "Method Not Allowed - Incorrect Host Header", http.StatusMethodNotAllowed)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream URL: %v", err)
	}

	rp, err := proxy.New(upstream.URL, nil)
	if err != nil {
		t.Fatalf("proxy.New failed: %v", err)
	}

	clientHost := "gw-da443dc4.elitegateway.site"
	req := httptest.NewRequest(http.MethodGet, "https://"+clientHost+"/api/addresses?page=1&limit=10", nil)
	req.Host = clientHost
	req.Header.Set("Authorization", "Bearer my-secret-token")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", res.StatusCode)
	}

	// 1. Verifies the upstream Host is used
	if receivedHost != upstreamURL.Host {
		t.Errorf("expected upstream Host %q, got %q", upstreamURL.Host, receivedHost)
	}

	// 2. Verifies X-Forwarded-Host preserves original client host
	if receivedForwardedHost != clientHost {
		t.Errorf("expected X-Forwarded-Host %q, got %q", clientHost, receivedForwardedHost)
	}

	// 3. Verifies GET remains GET
	if receivedMethod != http.MethodGet {
		t.Errorf("expected Method GET, got %q", receivedMethod)
	}

	// 4. Verifies path /api/addresses is preserved
	if receivedPath != "/api/addresses" {
		t.Errorf("expected Path /api/addresses, got %q", receivedPath)
	}

	// 5. Verifies query parameters are preserved
	if receivedQuery != "page=1&limit=10" {
		t.Errorf("expected Query %q, got %q", "page=1&limit=10", receivedQuery)
	}

	// 6. Verifies Authorization header is forwarded
	if receivedAuth != "Bearer my-secret-token" {
		t.Errorf("expected Authorization header %q, got %q", "Bearer my-secret-token", receivedAuth)
	}

	// 7. Verifies Content-Type header is preserved
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type %q, got %q", "application/json", receivedContentType)
	}

	_ = receivedBody
}

func TestReverseProxy_RejectsIncorrectHostHeader(t *testing.T) {
	var upstreamHost string

	// Upstream that rejects requests if Host header does NOT match upstream host
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != upstreamHost {
			http.Error(w, "Method Not Allowed - Host Header Mismatch", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer upstream.Close()

	u, _ := url.Parse(upstream.URL)
	upstreamHost = u.Host

	rp, err := proxy.New(upstream.URL, nil)
	if err != nil {
		t.Fatalf("proxy.New failed: %v", err)
	}

	clientHost := "gw-da443dc4.elitegateway.site"
	req := httptest.NewRequest(http.MethodGet, "https://"+clientHost+"/api/addresses", nil)
	req.Host = clientHost

	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK when proxy sets target host, got status %d", rec.Code)
	}
}

func TestReverseProxy_POSTWithRequestBody(t *testing.T) {
	var (
		receivedMethod string
		receivedBody   string
		receivedAuth   string
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)

		w.WriteHeader(http.StatusCreated)
	}))
	defer upstream.Close()

	rp, err := proxy.New(upstream.URL, nil)
	if err != nil {
		t.Fatalf("proxy.New failed: %v", err)
	}

	payload := `{"street":"123 Main St"}`
	req := httptest.NewRequest(http.MethodPost, "https://gw-da443dc4.elitegateway.site/api/addresses", strings.NewReader(payload))
	req.Host = "gw-da443dc4.elitegateway.site"
	req.Header.Set("Authorization", "Bearer post-token")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", rec.Code)
	}

	if receivedMethod != http.MethodPost {
		t.Errorf("expected POST method, got %q", receivedMethod)
	}

	if receivedBody != payload {
		t.Errorf("expected request body %q, got %q", payload, receivedBody)
	}

	if receivedAuth != "Bearer post-token" {
		t.Errorf("expected Authorization %q, got %q", "Bearer post-token", receivedAuth)
	}
}
