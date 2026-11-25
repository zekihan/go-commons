package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecoverHandler_Panic(t *testing.T) {
	handler := RecoverHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rr.Code)
	}
}

func TestRecoverHandler_Abort(t *testing.T) {
	handler := RecoverHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()

	defer func() {
		//nolint:err113,errorlint // Verify the original panic value is preserved.
		if got := recover(); got != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler", got)
		}

		if rr.Body.Len() != 0 {
			t.Errorf("response written during abort: %q", rr.Body.String())
		}
	}()

	handler.ServeHTTP(rr, req)
}

func TestRecoverHandler_NoPanic(t *testing.T) {
	called := false
	handler := RecoverHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected handler to be called")
	}

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected status 202, got %d", rr.Code)
	}
}

func TestRecoverHandler_PanicAfterWrite(t *testing.T) {
	for _, tc := range []struct {
		write func(http.ResponseWriter)
		name  string
	}{
		{name: "header", write: func(w http.ResponseWriter) { w.WriteHeader(http.StatusAccepted) }},
		{name: "body", write: func(w http.ResponseWriter) { _, _ = w.Write([]byte("partial")) }},
		{name: "flush", write: func(w http.ResponseWriter) { _ = http.NewResponseController(w).Flush() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := RecoverHandler(
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) { tc.write(w); panic("boom") },
				),
			)
			rr := httptest.NewRecorder()

			defer func() {
				//nolint:err113,errorlint // net/http recognizes the exact panic value.
				if got := recover(); got != http.ErrAbortHandler {
					t.Errorf("recovered %v, want http.ErrAbortHandler", got)
				}

				if rr.Body.String() == "Internal Server Error\n" {
					t.Error("wrote an error response after committing")
				}
			}()

			handler.ServeHTTP(
				rr,
				httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody),
			)
		})
	}
}
