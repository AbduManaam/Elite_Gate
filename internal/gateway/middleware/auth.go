package middleware

import (
    "log"
    "net/http"
)

func Auth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

        apiKey := r.Header.Get("X-API-Key")

        if apiKey == "" {
            http.Error(w, "missing api key", http.StatusUnauthorized)
            return
        }

        // Stub validation
        if apiKey != "test-key" {
            http.Error(w, "invalid api key", http.StatusUnauthorized)
            return
        }

        log.Println("Auth passed")

        next.ServeHTTP(w, r)
    })
}