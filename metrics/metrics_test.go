package metrics

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func scrape(t *testing.T, registry *Registry) string {
	t.Helper()

	recorder := httptest.NewRecorder()
	registry.Handler().ServeHTTP(
		recorder,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", http.NoBody),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	return recorder.Body.String()
}

func TestHandlerIncludesRuntimeAndProcessMetrics(t *testing.T) {
	registry := New()
	registry.Counter("app_jobs_total", Labels{"kind": "sync"}).Inc()

	body := scrape(t, registry)

	for _, want := range []string{
		`app_jobs_total{kind="sync"} 1`,
		"go_goroutines",
		"go_memstats_alloc_bytes",
		"process_resident_memory_bytes",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected scrape to contain %q", want)
		}
	}
}

func TestCounterGaugeHistogram(t *testing.T) {
	registry := New()

	counter := registry.Counter("app_jobs_total", Labels{"kind": "sync"})
	counter.Inc()
	counter.Add(2)

	if got := registry.Counter("app_jobs_total", Labels{"kind": "sync"}).Get(); got != 3 {
		t.Errorf("expected shared counter value 3, got %d", got)
	}

	gauge := registry.Gauge("app_queue_depth", nil)
	gauge.Set(7)
	gauge.Add(-2)

	if got := registry.Gauge("app_queue_depth", nil).Get(); got != 5 {
		t.Errorf("expected shared gauge value 5, got %v", got)
	}

	registry.Histogram("app_job_duration_seconds", nil).Observe(0.25)

	body := scrape(t, registry)
	for _, want := range []string{
		`app_jobs_total{kind="sync"} 3`,
		"app_queue_depth 5",
		"app_job_duration_seconds_bucket",
		"app_job_duration_seconds_count",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected scrape to contain %q, got:\n%s", want, body)
		}
	}
}

func TestInvalidNamesPanic(t *testing.T) {
	registry := New()

	for _, call := range []func(){
		func() { registry.Counter("bad-name", nil) },
		func() { registry.Counter("ok_total", Labels{"bad-label": "x"}) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected panic for invalid name")
				}
			}()

			call()
		}()
	}
}

func TestMiddlewareUsesServeMuxPattern(t *testing.T) {
	registry := New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	})
	handler := registry.Middleware(mux)

	for range 2 {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequestWithContext(
			t.Context(), http.MethodGet, "/users/12345", http.NoBody,
		))

		if recorder.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", recorder.Code)
		}
	}

	body := scrape(t, registry)
	for _, want := range []string{
		`http_requests_total{method="GET",route="/users/{id}",status="201"} 2`,
		`http_request_duration_seconds_bucket{method="GET",route="/users/{id}",status="201"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected scrape to contain %q, got:\n%s", want, body)
		}
	}

	if strings.Contains(body, "/users/12345") {
		t.Error("raw path leaked into labels; cardinality is not bounded")
	}
}

func TestMiddlewareStatusCapture(t *testing.T) {
	registry := New()
	handler := registry.Middleware(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("ok")) // Implicit 200.
		},
	))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/anything/at/all", http.NoBody,
	))

	body := scrape(t, registry)

	want := `http_requests_total{method="POST",route="unknown",status="200"} 1`
	if !strings.Contains(body, want) {
		t.Errorf("expected scrape to contain %q, got:\n%s", want, body)
	}

	if strings.Contains(body, "/anything/at/all") {
		t.Error("unrouted raw path leaked into labels")
	}
}

func TestMiddlewareWithRoute(t *testing.T) {
	registry := New()
	handler := registry.MiddlewareWithRoute("/report/{id}", http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusTeapot)
		},
	))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/report/999", http.NoBody,
	))

	body := scrape(t, registry)

	want := `http_requests_total{method="GET",route="/report/{id}",status="418"} 1`
	if !strings.Contains(body, want) {
		t.Errorf("expected scrape to contain %q, got:\n%s", want, body)
	}
}

func TestListenAndServe(t *testing.T) {
	registry := New()
	registry.Counter("app_ready_total", nil).Inc()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}

	addr := listener.Addr().String()
	_ = listener.Close()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() { done <- registry.ListenAndServe(ctx, addr) }()

	deadline := time.Now().Add(5 * time.Second)

	for {
		//nolint:noctx // Test probe; the client has no request-scoped context to reuse.
		response, getErr := http.Get("http://" + addr + "/metrics")
		if getErr == nil {
			body := make([]byte, 4096)
			count, _ := response.Body.Read(body)
			_ = response.Body.Close()

			if !strings.Contains(string(body[:count]), "app_ready_total 1") {
				t.Error("separate server did not serve registry metrics")
			}

			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("metrics server never came up: %v", getErr)
		}

		time.Sleep(10 * time.Millisecond) //nolint:forbidigo // Poll test server startup.
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil after cancel, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("server did not shut down after context cancellation")
	}
}

func TestMiddlewareBoundsUnknownMethods(t *testing.T) {
	registry := New()
	handler := registry.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	request := httptest.NewRequestWithContext(t.Context(), "BREW", "/", http.NoBody)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	var output strings.Builder
	registry.WritePrometheus(&output)

	want := `http_requests_total{method="other",route="unknown",status="200"} 1`
	if !strings.Contains(output.String(), want) {
		t.Fatalf("output missing %q:\n%s", want, output.String())
	}
}
