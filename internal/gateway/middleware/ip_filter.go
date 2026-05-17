package middleware

import (
    "net/http"
)

var blockedIPs = map[string]bool{
    "192.168.1.10": true,
}

func IPFilter(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

        ip := r.RemoteAddr

        if blockedIPs[ip] {
            http.Error(w, "ip blocked", http.StatusForbidden)
            return
        }

        next.ServeHTTP(w, r)
    })
}