package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestLogging_HandlerCalled(t *testing.T) {
	var logBuffer bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuffer, nil))) // capture logs in buffer

	called := false
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/test", http.NoBody)
	req.RemoteAddr = "192.0.2.100:12345" // simulate a request from a specific IP
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected wrapped handler to be called")
	}

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rr.Code)
	}

	logStr := logBuffer.String()
	checkLogs(t, logStr, http.MethodPost, "192.0.2.100", http.StatusCreated, "/test")

	if !strings.Contains(logStr, "hasQuery=false") {
		t.Errorf("expected absent query metadata, got %s", logStr)
	}
}

func TestLogging_HandlerCalled_IPUnknown(t *testing.T) {
	var logBuffer bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuffer, nil))) // capture logs in buffer

	called := false
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/test", http.NoBody)
	req.RemoteAddr = "300.0.113.1:1234" // simulate invalid IP
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected wrapped handler to be called")
	}

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}

	logStr := logBuffer.String()
	checkLogs(t, logStr, http.MethodPost, "unknown", http.StatusCreated, "/test")
}

func TestLogging_SkippedPaths(t *testing.T) {
	paths := []string{"/ping", "/health", "/ready", "/api/ping", "/api/health", "/api/ready"}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			var logBuffer bytes.Buffer
			slog.SetDefault(slog.New(slog.NewTextHandler(&logBuffer, nil)))

			called := false
			handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true

				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
			req.RemoteAddr = "127.0.0.1:1234"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if !called {
				t.Errorf("expected handler to be called for skipped path %s", path)
			}

			if strings.Contains(logBuffer.String(), "HTTP request") {
				t.Errorf(
					"expected no log entry for skipped path %s, got: %s",
					path,
					logBuffer.String(),
				)
			}
		})
	}
}

func TestLogging_SkippedContains(t *testing.T) {
	var logBuffer bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuffer, nil)))

	called := false
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

		w.WriteHeader(http.StatusOK)
	}))

	// defaultSkippedContains matches the literal path "favicon.ico" via exact equality,
	// so we set URL.Path directly rather than passing it as the request URL.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	req.URL.Path = "favicon.ico"
	req.RemoteAddr = "127.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected handler to be called for favicon.ico path")
	}

	if strings.Contains(logBuffer.String(), "HTTP request") {
		t.Errorf("expected no log entry for favicon.ico, got: %s", logBuffer.String())
	}
}

func checkLogs(t *testing.T, logStr, method, ip string, status int, path string) {
	t.Helper()

	if !strings.Contains(logStr, method) {
		t.Errorf("expected log to contain method '%s', got %s", method, logStr)
	}

	if !strings.Contains(logStr, ip) {
		t.Errorf("expected log to contain IP '%s', got %s", ip, logStr)
	}

	if !strings.Contains(logStr, strconv.Itoa(status)) {
		t.Errorf("expected log to contain status '%d', got %s", status, logStr)
	}

	if !strings.Contains(logStr, path) {
		t.Errorf("expected log to contain path '%s', got %s", path, logStr)
	}
}

func TestLogging_QueryPrivacy(t *testing.T) {
	queries := []string{
		"apiKey=SECRET_SENTINEL",
		"api_key=SECRET_SENTINEL&APIKEY=SECOND_SENTINEL",
		"access_token=SECRET_SENTINEL&refreshToken=SECOND_SENTINEL",
		"code=SECRET_SENTINEL&authorization_code=SECOND_SENTINEL",
		"password=SECRET_SENTINEL&passwd=SECOND_SENTINEL&pwd=THIRD_SENTINEL",
		"%61piKey=%53ECRET_SENTINEL&apiKey=SECOND_SENTINEL",
		"apiKey=SECRET_SENTINEL;password=SECOND_SENTINEL&broken=%zz",
		"unexpected=SECRET_SENTINEL&SECRET_SENTINEL=value",
		"filter=unread&page=2",
		"",
	}

	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			var buffer bytes.Buffer

			previous := slog.Default()

			slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })

			request := httptest.NewRequestWithContext(
				t.Context(), http.MethodGet, "/reports?"+query, http.NoBody,
			)
			request.RemoteAddr = "192.0.2.1:1234"
			handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RawQuery != query {
					t.Errorf("middleware changed the query: %q", r.URL.RawQuery)
				}

				w.WriteHeader(http.StatusAccepted)
			}))
			handler.ServeHTTP(httptest.NewRecorder(), request)

			logOutput := buffer.String()
			if strings.Contains(logOutput, "SENTINEL") || strings.Contains(logOutput, "unread") {
				t.Fatalf("query contents leaked into logs: %s", logOutput)
			}

			var record map[string]any

			err := json.Unmarshal(buffer.Bytes(), &record)
			if err != nil {
				t.Fatal(err)
			}

			if _, exists := record["query"]; exists {
				t.Error("raw query must not be logged")
			}

			if record["hasQuery"] != true {
				t.Error("expected query presence metadata, including an empty trailing query")
			}

			checkLogs(t, logOutput, http.MethodGet, "192.0.2.1", http.StatusAccepted, "/reports")
		})
	}
}
