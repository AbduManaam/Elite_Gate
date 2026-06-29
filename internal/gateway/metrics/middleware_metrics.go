package metrics

import (
	"net/http"
	"strconv"
	"time"

	"elitegate/internal/model"
	"elitegate/internal/shared"
)

// responseWriterWrapper wraps http.ResponseWriter to capture the HTTP status code
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	return rw.ResponseWriter.Write(b)
}

// Middleware records request counts, latencies, and active connections
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		path := r.URL.Path
		method := r.Method
		upstreamName := "unknown"

		// Try to read route details from context
		if rt, ok := r.Context().Value(shared.ContextKeyRoute).(*model.Route); ok && rt != nil {
			path = rt.Path // Use matched route path pattern (e.g. "/api/users") instead of dynamic URL
			if rt.UpstreamID != nil {
				upstreamName = *rt.UpstreamID
			} else if rt.UpstreamURL != "" {
				upstreamName = rt.UpstreamURL
			}
		}

		ActiveRequests.WithLabelValues(path, method).Inc()
		defer ActiveRequests.WithLabelValues(path, method).Dec()

		rw := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		statusStr := strconv.Itoa(rw.statusCode)

		HTTPRequestCounter.WithLabelValues(path, method, statusStr, upstreamName).Inc()
		HTTPRequestDuration.WithLabelValues(path, method, statusStr, upstreamName).Observe(duration)
	})
}
