package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
)

// RequestLogger logs HTTP requests with structured logging.
func RequestLogger(logger *observability.Logger, metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				duration := time.Since(start)
				status := ww.Status()
				reqID := GetRequestID(r.Context())
				if reqID == "" {
					reqID = middleware.GetReqID(r.Context())
				}

				logger.Info("http_request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"duration_ms", duration.Milliseconds(),
					"request_id", reqID,
				)

				if metrics != nil {
					metrics.ObserveRequestDuration(r.Method, r.URL.Path, status, duration)
					if status >= 500 {
						metrics.IncRequestErrors(r.Method, r.URL.Path)
					}
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
