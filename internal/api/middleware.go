package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type HTTPMetricsRecorder interface {
	IncHTTPRequest(pattern, method string, statusCode int)
}

type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterInterceptor) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func MetricsMiddleware(m HTTPMetricsRecorder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wrapper := &responseWriterInterceptor{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(wrapper, r)

			if m == nil {
				return
			}

			routePattern := "unknown"
			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				if p := rctx.RoutePattern(); p != "" {
					routePattern = p
				}
			}

			m.IncHTTPRequest(routePattern, r.Method, wrapper.statusCode)
		})
	}
}
