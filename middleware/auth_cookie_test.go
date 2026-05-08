package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zekihan/go-commons/cookie"
	"github.com/zekihan/go-commons/session"
)

func TestAuthHandler_NoAuthEnabled(t *testing.T) {
	AuthHandler := GetAuthMiddlewareForCookie(false, "")
	handler := AuthHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestAuthHandler_AuthEnabled_Unauthenticated(t *testing.T) {
	AuthHandler := GetAuthMiddlewareForCookie(true, "cookieKey")
	handler := AuthHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	if loc := rec.Header().Get("Location"); loc == "" || !strings.Contains(loc, "/login") {
		t.Errorf("expected Location header to contain '/login', got %q", loc)
	}
}

func TestAuthHandler_AuthEnabled_Authenticated(t *testing.T) {
	AuthHandler := GetAuthMiddlewareForCookie(true, "cookieKey")
	handler := AuthHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	sessionCookieObj := session.Cookie{
		Authenticated: true,
		ExpireTime:    time.Now().Add(time.Hour),
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	sessionCookie, err := cookie.CreateNewCookie("cookieKey", sessionCookieObj)
	if err != nil {
		t.Fatalf("Failed to create cookie: %v", err)
	}

	req.AddCookie(sessionCookie)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRedirect(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/some-path", http.NoBody)
	rec := httptest.NewRecorder()

	redirect(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	if loc := rec.Header().Get("Location"); loc == "" ||
		!strings.Contains(loc, "/login?redirect=/some-path") {
		t.Errorf("expected Location header to contain '/login?redirect=/some-path', got %q", loc)
	}
}

func TestRedirectFromHx(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/some-path", http.NoBody)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()

	redirect(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	if val := rec.Header().Get("HX-Refresh"); val == "" || !strings.Contains(val, "true") {
		t.Errorf("expected HX-Refresh header to contain 'true', got %q", val)
	}
}

func TestGetCookie_ValidCookie(t *testing.T) {
	sessionCookieObj := session.Cookie{
		Authenticated: true,
		ExpireTime:    time.Now().Add(-time.Hour),
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	sessionCookie, err := cookie.CreateNewCookie("cookieKey", sessionCookieObj)
	if err != nil {
		t.Fatalf("Failed to create cookie: %v", err)
	}

	req.AddCookie(sessionCookie)

	result, ok := getCookie(req, "cookieKey")
	if !ok {
		t.Errorf("expected ok to be true, got false")
	}

	if result.Authenticated != sessionCookieObj.Authenticated {
		t.Errorf(
			"expected Authenticated to be %v, got %v",
			sessionCookieObj.Authenticated,
			result.Authenticated,
		)
	}
}

func TestGetCookie_InvalidCookie(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	result, ok := getCookie(req, "")
	if ok {
		t.Errorf("expected ok to be false, got true")
	}

	if result != nil {
		t.Errorf("expected result to be nil, got %v", result)
	}
}
