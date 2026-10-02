package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
)

// Recovery recovers from panics and returns a 500 error.
func Recovery(logger *observability.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					reqID := GetRequestID(r.Context())
					logger.Error("panic recovered",
						"error", fmt.Sprintf("%v", err),
						"request_id", reqID,
						"method", r.Method,
						"path", r.URL.Path,
						"stack", string(debug.Stack()),
					)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
