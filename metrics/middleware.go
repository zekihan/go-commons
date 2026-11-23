package metrics

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
)

type (
	routeContextKey  struct{}
	holderContextKey struct{}
)

// routeHolder lets handlers deeper in the chain report the route back to
// Middleware even when intermediate middlewares replace the request with a
// copy (for example through Request.WithContext).
type routeHolder struct {
	route string
}

// WithRoute returns a copy of request carrying an explicit route name for
// Middleware. Use it when the route is not a Go 1.22 ServeMux pattern, for
// example behind another router.
func WithRoute(request *http.Request, route string) *http.Request {
	if holder, ok := request.Context().Value(holderContextKey{}).(*routeHolder); ok {
		holder.route = route
	}

	return request.WithContext(context.WithValue(request.Context(), routeContextKey{}, route))
}

// CapturePattern reports the Go 1.22 ServeMux pattern back to Middleware.
// Wrap the ServeMux itself with it when middlewares between Middleware and
// the mux copy the request, since the mux sets Pattern only on its copy:
//
//	srv.Handler = registry.Middleware(logRequests(metrics.CapturePattern(mux)))
func CapturePattern(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		next.ServeHTTP(writer, request)

		holder, ok := request.Context().Value(holderContextKey{}).(*routeHolder)
		if ok && holder.route == "" && request.Pattern != "" {
			holder.route = patternPath(request.Pattern)
		}
	})
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
		holder := &routeHolder{}
		request = request.WithContext(
			context.WithValue(request.Context(), holderContextKey{}, holder),
		)
		captured := httpsnoop.CaptureMetrics(next, writer, request)
		status := captured.Code

		if status == 0 {
			status = http.StatusOK
		}

		routeName := route
		if routeName == "" {
			routeName = routeLabel(request, holder)
		}

		labels := Labels{
			"method": methodLabel(request.Method),
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

func methodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method
	default:
		return "other"
	}
}

func routeLabel(request *http.Request, holder *routeHolder) string {
	if route, ok := request.Context().Value(routeContextKey{}).(string); ok && route != "" {
		return route
	}

	if holder.route != "" {
		return holder.route
	}

	if request.Pattern != "" {
		return patternPath(request.Pattern)
	}

	return "unknown"
}

// patternPath strips the method from "METHOD /path", keeping "/path".
func patternPath(pattern string) string {
	if _, path, found := strings.Cut(pattern, " "); found {
		return path
	}

	return pattern
}
