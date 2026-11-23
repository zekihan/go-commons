package metrics

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
)

type routeContextKey struct{}

// WithRoute returns a copy of request carrying an explicit route name for
// Middleware. Use it when the route is not a Go 1.22 ServeMux pattern, for
// example behind another router.
func WithRoute(request *http.Request, route string) *http.Request {
	return request.WithContext(context.WithValue(request.Context(), routeContextKey{}, route))
}

// Middleware records http_requests_total and
// http_request_duration_seconds, labeled by method, route, and status.
//
// The route label comes from, in order: WithRoute, the Go 1.22 ServeMux
// pattern matched for the request (r.Pattern, with any method prefix
// removed), or "unknown". Raw URL paths are never used, so label
// cardinality stays bounded by the application's route table. Mount it
// around the ServeMux so the pattern is known:
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("GET /users/{id}", handler)
//	srv.Handler = registry.Middleware(mux)
func (registry *Registry) Middleware(next http.Handler) http.Handler {
	return registry.middleware("", next)
}

func (registry *Registry) middleware(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := time.Now()
		captured := httpsnoop.CaptureMetrics(next, writer, request)
		status := captured.Code

		if status == 0 {
			status = http.StatusOK
		}

		routeName := route
		if routeName == "" {
			routeName = routeLabel(request)
		}

		labels := Labels{
			"method": request.Method,
			"route":  routeName,
			"status": strconv.Itoa(status),
		}
		registry.Counter("http_requests_total", labels).Inc()
		registry.Histogram("http_request_duration_seconds", labels).
			Observe(time.Since(start).Seconds())
	})
}

// MiddlewareWithRoute is Middleware with a fixed route name, for handlers
// that are not routed by a Go 1.22 ServeMux:
//
//	mux.Handle("/report", registry.MiddlewareWithRoute("/report", handler))
func (registry *Registry) MiddlewareWithRoute(route string, next http.Handler) http.Handler {
	return registry.middleware(route, next)
}

func routeLabel(request *http.Request) string {
	if route, ok := request.Context().Value(routeContextKey{}).(string); ok && route != "" {
		return route
	}

	if request.Pattern != "" {
		// Pattern is "METHOD /path" or "/path"; keep only the path part.
		if _, path, found := strings.Cut(request.Pattern, " "); found {
			return path
		}

		return request.Pattern
	}

	return "unknown"
}
