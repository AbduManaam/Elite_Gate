package middleware

import (
	"elitegate/internal/ipfilter"
	"net/http"
)

var blockedIPs = map[string]bool{
	"192.168.1.10": true,
}

func IPFilter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ipfilter.ExtractIP(r.RemoteAddr, r.Header.Get, false)
		if blockedIPs[ip] {
			http.Error(w, "ip blocked", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
