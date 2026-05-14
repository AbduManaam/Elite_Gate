package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
)

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "service":      "test-backend",
            "method":       r.Method,
            "path":         r.URL.Path,
            "received":     true,
            "forwarded_by": r.Header.Get("X-Gateway"),
        })
    })
    fmt.Println("Test backend running on :9090")
    log.Fatal(http.ListenAndServe(":9090", mux))
}