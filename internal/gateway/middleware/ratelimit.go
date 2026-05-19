package middleware

import (
    "net"
    "net/http"
    "sync"
    "time"
)

type clientCounter struct {
    count     int
    expiresAt time.Time
}

var (
    mu       sync.Mutex
    counters = make(map[string]*clientCounter)
)

func RateLimit(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

        ip, _, err := net.SplitHostPort(r.RemoteAddr)
        if err != nil {
            ip = r.RemoteAddr
        }

        mu.Lock()

        counter, exists := counters[ip]

        if !exists || time.Now().After(counter.expiresAt) {
            counter = &clientCounter{
                count:     0,
                expiresAt: time.Now().Add(time.Minute),
            }
            counters[ip] = counter
        }

        counter.count++

        if counter.count > 10 {
            mu.Unlock()

            http.Error(
                w,
                "rate limit exceeded",
                http.StatusTooManyRequests,
            )
            return
        }

        mu.Unlock()

        next.ServeHTTP(w, r)
    })
}