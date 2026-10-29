package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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

	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}

	if location.Path != "/login" || location.Query().Get("redirect") != "/some-path" {
		t.Errorf("unexpected login redirect: %s", location)
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

func TestAuthHandler_EmptyCookieKey(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		for _, htmx := range []bool{true, false} {
			called := false
			handler := GetAuthMiddlewareForCookie(enabled, "")(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true

					w.WriteHeader(http.StatusOK)
				}),
			)

			request := httptest.NewRequestWithContext(
				t.Context(), http.MethodGet, "/private", http.NoBody,
			)
			if htmx {
				request.Header.Set("HX-Request", "true")
			}

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			expectedStatus := http.StatusOK
			if enabled {
				expectedStatus = http.StatusSeeOther
				if htmx {
					expectedStatus = http.StatusUnauthorized
				}
			}

			if called == enabled || recorder.Code != expectedStatus {
				t.Errorf(
					"enabled=%t htmx=%t: handler called=%t, status=%d; want called=%t, status=%d",
					enabled,
					htmx,
					called,
					recorder.Code,
					!enabled,
					expectedStatus,
				)
			}

			if enabled && htmx && recorder.Header().Get("HX-Refresh") != "true" {
				t.Error("expected HX-Refresh for rejected HTMX request")
			}
		}
	}
}

func TestAuthHandler_RedirectRoundTrip(t *testing.T) {
	targets := []string{
		"/reports/a%26b?filter=unread",
		"/reports/a%2Fb?filter=a%26b&filter=second&next=%2Fprivate",
		"/reports/a%23b?code=a%2Bb&empty=",
		"/reports/%E2%9C%93?search=hello+world",
		"/reports?",
		"https://example.test/reports/a%26b?filter=unread",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			request := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodGet,
				target,
				http.NoBody,
			)
			recorder := httptest.NewRecorder()
			handler := GetAuthMiddlewareForCookie(true, "session")(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Error("unauthenticated request reached protected handler")
				}),
			)
			handler.ServeHTTP(recorder, request)

			location, err := url.Parse(recorder.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}

			if recorder.Code != http.StatusSeeOther || location.Path != "/login" ||
				location.IsAbs() || location.Host != "" {
				t.Fatalf("unexpected redirect: status=%d location=%s", recorder.Code, location)
			}

			if got := location.Query().Get("redirect"); got != request.URL.RequestURI() {
				t.Errorf("redirect=%q; want %q", got, request.URL.RequestURI())
			}

			if len(location.Query()) != 1 {
				t.Errorf("request query escaped into login parameters: %s", location.RawQuery)
			}
		})
	}
}

func TestRedirect_UnsafeReturnTargets(t *testing.T) {
	targets := []string{
		"//example.test/private",
		"/%2Fexample.test/private",
		"/%5Cexample.test/private",
		"/%09/example.test/private",
		"/%0D%0ALocation:example.test",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			request := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodGet,
				target,
				http.NoBody,
			)
			recorder := httptest.NewRecorder()
			redirect(recorder, request)

			if location := recorder.Header().Get("Location"); location != "/login" {
				t.Errorf("unsafe return target retained: %q", location)
			}
		})
	}
}
