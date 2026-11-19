package ready

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPing(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ready", http.NoBody)
	rr := httptest.NewRecorder()
	rr.Header().Set("Cache-Control", "public, max-age=60")
	GetReady(rr, req)

	// Verify status code.
	if rr.Code != http.StatusOK {
		t.Errorf("GetPing returned wrong status code: got %d want %d", rr.Code, http.StatusOK)
	}

	// Verify body.
	expected := "OK\n"
	if rr.Body.String() != expected {
		t.Errorf("GetPing returned unexpected body: got %s want %s", rr.Body.String(), expected)
	}

	res := rr.Result()
	defer res.Body.Close()

	if got := res.Header.Values("Cache-Control"); len(got) != 1 || got[0] != "no-store" {
		t.Errorf("Cache-Control = %q, want [no-store]", got)
	}
}
