package errorroutes

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zekihan/go-commons/web"
)

func TestSendErrorMessageContentTypes(t *testing.T) {
	tests := []struct {
		name        string
		accept      string
		contentType string
		body        string
		status      int
	}{
		{
			name:        "html",
			accept:      "text/html",
			status:      http.StatusTeapot,
			contentType: "text/html; charset=utf-8",
			body:        "<html><body><h1>418 I'm a teapot</h1></body></html>",
		},
		{
			name:        "json",
			accept:      "application/json",
			status:      http.StatusTeapot,
			contentType: "application/json",
			body:        `{"error": "I'm a teapot"}`,
		},
		{
			name:        "plain",
			accept:      "text/plain",
			status:      http.StatusTeapot,
			contentType: "text/plain",
			body:        "I'm a teapot",
		},
		{
			name:        "none",
			accept:      "",
			status:      http.StatusTeapot,
			contentType: "text/plain",
			body:        "I'm a teapot",
		},
		{
			name:        "unsupported media type",
			accept:      "application/xml",
			status:      599,
			contentType: "text/plain",
			body:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodGet,
				"/missing",
				http.NoBody,
			)
			if tt.accept != "" {
				request.Header.Set("Accept", tt.accept)
			}

			recorder := httptest.NewRecorder()
			SendErrorMessage(recorder, request, tt.status)

			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.status)
			}

			if got := recorder.Header().Get("Content-Type"); got != tt.contentType {
				t.Fatalf("Content-Type = %q, want %q", got, tt.contentType)
			}

			if got := recorder.Header().Get("Vary"); got != "Accept" {
				t.Fatalf("Vary = %q", got)
			}

			if recorder.Body.String() != tt.body {
				t.Fatalf("body = %q, want %q", recorder.Body.String(), tt.body)
			}

			if web.GetAcceptType(request) == "" {
				t.Fatal("GetAcceptType returned the zero value")
			}
		})
	}
}

func TestSendErrorMessageWriteFailure(t *testing.T) {
	accepts := []string{"text/html", "application/json", "text/plain", "application/xml", ""}

	for _, accept := range accepts {
		t.Run(accept, func(t *testing.T) {
			request := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodGet,
				"/missing",
				http.NoBody,
			)
			if accept != "" {
				request.Header.Set("Accept", accept)
			}

			recorder := httptest.NewRecorder()

			SendErrorMessage(failingWriter{ResponseWriter: recorder}, request, http.StatusNotFound)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf(
					"status = %d, want the status committed before the body write",
					recorder.Code,
				)
			}
		})
	}
}

type failingWriter struct {
	http.ResponseWriter
}

var errBrokenConnection = errors.New("broken connection")

func (failingWriter) Write([]byte) (int, error) {
	return 0, errBrokenConnection
}
