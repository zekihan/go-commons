package cookie

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateNewCookie(t *testing.T) {
	cookie, err := CreateNewCookie("testCookie", "testValue", "strict", "/testpath")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cookie == nil {
		t.Fatal("expected cookie to be non-nil")
	}

	if cookie.Name != "testCookie" {
		t.Errorf("expected cookie name to be 'testCookie', got %s", cookie.Name)
	}

	if cookie.Path != "/testpath" {
		t.Errorf("expected cookie path to be '/testpath', got %s", cookie.Path)
	}

	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite to be Strict, got %v", cookie.SameSite)
	}

	if !cookie.Secure {
		t.Error("expected cookie to be secure")
	}

	if !cookie.HttpOnly {
		t.Error("expected cookie to be HttpOnly")
	}
}

func TestSetCookie(t *testing.T) {
	rec := httptest.NewRecorder()

	err := SetCookie(rec, nil, "testCookie", "testValue", "lax")
	if err != nil {
		t.Fatalf("SetCookie returned error: %v", err)
	}

	resp := rec.Result()
	cookie := resp.Cookies()[0]

	if cookie.Name != "testCookie" {
		t.Errorf("expected cookie name to be 'testCookie', got %s", cookie.Name)
	}

	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite to be Lax, got %v", cookie.SameSite)
	}

	if !cookie.Secure {
		t.Error("expected cookie to be secure")
	}

	if !cookie.HttpOnly {
		t.Error("expected cookie to be HttpOnly")
	}
}

func TestGetCookie(t *testing.T) {
	// Create a cookie
	rec := httptest.NewRecorder()

	err := SetCookie(rec, nil, "testCookie", "testValue", "lax")
	if err != nil {
		t.Fatalf("SetCookie returned error: %v", err)
	}

	resp := rec.Result()
	cookie := resp.Cookies()[0]

	// Mock a request with the cookie
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	req.AddCookie(cookie)

	var value string

	err = GetCookie(req, "testCookie", &value)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if value != "testValue" {
		t.Errorf("expected cookie value to be 'testValue', got %s", value)
	}
}

func TestCookieRejectsInvalidAttributes(t *testing.T) {
	tests := []struct {
		label string
		name  string
		path  string
	}{
		{label: "empty name", name: "", path: "/"},
		{label: "space in name", name: "bad name", path: "/"},
		{label: "delimiter in name", name: "bad;name", path: "/"},
		{label: "newline in name", name: "bad\nname", path: "/"},
		{label: "semicolon in path", name: "session", path: "/private;admin"},
		{label: "newline in path", name: "session", path: "/private\nadmin"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			cookie, err := CreateNewCookie(tt.name, "value", "lax", tt.path)
			if err == nil || cookie != nil {
				t.Errorf("CreateNewCookie = (%v, %v), want nil cookie and error", cookie, err)
			}

			rec := httptest.NewRecorder()

			err = SetCookie(rec, nil, tt.name, "value", "lax", tt.path)
			if err == nil {
				t.Error("SetCookie returned success for invalid attributes")
			}

			if headers := rec.Header().Values("Set-Cookie"); len(headers) != 0 {
				t.Errorf("SetCookie emitted headers for invalid attributes: %v", headers)
			}
		})
	}
}
