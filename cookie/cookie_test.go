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
	w := httptest.NewRecorder()
	SetCookie(w, nil, "testCookie", "testValue", "lax")

	resp := w.Result()
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
	w := httptest.NewRecorder()
	SetCookie(w, nil, "testCookie", "testValue", "lax")
	resp := w.Result()
	cookie := resp.Cookies()[0]

	// Mock a request with the cookie
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.AddCookie(cookie)

	var value string

	err := GetCookie(req, "testCookie", &value)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if value != "testValue" {
		t.Errorf("expected cookie value to be 'testValue', got %s", value)
	}
}
