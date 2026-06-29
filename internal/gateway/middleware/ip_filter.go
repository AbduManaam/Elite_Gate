package middleware

import (
	"elitegate/internal/ipfilter"
	"net/http"
)

var blockedIPs *ipfilter.IPChecker

// Supports CIDR Range
func init() {
	var err error
	blockedIPs, err = ipfilter.NewIPChecker([]string{})
	if err != nil {
		panic("failed to initialize IPFilter checker: " + err.Error())
	}
}

func IPFilter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ipfilter.ExtractIP(r.RemoteAddr, r.Header.Get, false)
		if blockedIPs.IsBlocked(ip) {
			http.Error(w, "ip blocked", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
