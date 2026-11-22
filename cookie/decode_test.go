package cookie

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestCreateNewCookieSameSiteAndPath(t *testing.T) {
	const day = 24 * time.Hour

	tests := []struct {
		name     string
		path     string
		options  []string
		sameSite http.SameSite
	}{
		{name: "default", path: "/", sameSite: http.SameSiteLaxMode},
		{name: "explicit lax", options: []string{"lax"}, path: "/", sameSite: http.SameSiteLaxMode},
		{name: "empty option", options: []string{""}, path: "/", sameSite: http.SameSiteLaxMode},
		{
			name:     "unknown option",
			options:  []string{"Strict"},
			path:     "/",
			sameSite: http.SameSiteLaxMode,
		},
		{name: "none", options: []string{"none"}, path: "/", sameSite: http.SameSiteNoneMode},
		{
			name:     "none with path",
			options:  []string{"none", "/app", "ignored"},
			path:     "/app",
			sameSite: http.SameSiteNoneMode,
		},
		{
			name:     "strict with path",
			options:  []string{"strict", "/admin"},
			path:     "/admin",
			sameSite: http.SameSiteStrictMode,
		},
		{
			name:     "empty path",
			options:  []string{"lax", ""},
			path:     "",
			sameSite: http.SameSiteLaxMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issued, err := CreateNewCookie("session", "value", tt.options...)
			if err != nil {
				t.Fatalf("CreateNewCookie: %v", err)
			}

			if issued.SameSite != tt.sameSite || issued.Path != tt.path {
				t.Fatalf("SameSite=%v Path=%q, want SameSite=%v Path=%q",
					issued.SameSite, issued.Path, tt.sameSite, tt.path)
			}

			if !issued.Secure || !issued.HttpOnly || !issued.Expires.IsZero() {
				t.Fatalf("cookie flags = secure:%t httpOnly:%t expires:%s",
					issued.Secure, issued.HttpOnly, issued.Expires)
			}

			if issued.MaxAge != int(day.Seconds()) {
				t.Fatalf("MaxAge = %d, want %d", issued.MaxAge, int(day.Seconds()))
			}
		})
	}
}

func TestCookieStructRoundTrip(t *testing.T) {
	issued := map[string]any{"id": "abc", "n": 7}

	signed, err := CreateNewCookie("payload", issued, "strict", "/account")
	if err != nil {
		t.Fatalf("CreateNewCookie: %v", err)
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	request.AddCookie(signed)

	var loaded map[string]any

	err = GetCookie(request, "payload", &loaded)
	if err != nil {
		t.Fatalf("GetCookie: %v", err)
	}

	if loaded["id"] != "abc" || loaded["n"] != 7 {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestGetCookieRejectsMissingTamperedAndReboundValues(t *testing.T) {
	signed, err := CreateNewCookie("session", "secret")
	if err != nil {
		t.Fatalf("CreateNewCookie: %v", err)
	}

	missing := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	var loaded string

	err = GetCookie(missing, "session", &loaded)
	if !errors.Is(err, http.ErrNoCookie) {
		t.Fatalf("missing cookie error = %v", err)
	}

	empty := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	empty.AddCookie(&http.Cookie{
		Name:     "session",
		Value:    "",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	loaded = "sentinel"

	err = GetCookie(empty, "session", &loaded)
	if err == nil || errors.Is(err, http.ErrNoCookie) {
		t.Fatalf("empty cookie error = %v", err)
	}

	if loaded != "sentinel" {
		t.Fatalf("empty cookie wrote %q", loaded)
	}

	tamperedRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	tamperedRequest.AddCookie(&http.Cookie{
		Name:     signed.Name,
		Value:    flipEncodedValue(signed.Value),
		Path:     signed.Path,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	err = GetCookie(tamperedRequest, "session", &loaded)
	if err == nil {
		t.Fatal("tampered cookie was accepted")
	}

	if loaded != "sentinel" {
		t.Fatalf("tampered cookie wrote %q", loaded)
	}

	renamedRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	renamedRequest.AddCookie(&http.Cookie{
		Name:     "other",
		Value:    signed.Value,
		Path:     signed.Path,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	err = GetCookie(renamedRequest, "other", &loaded)
	if err == nil {
		t.Fatal("cookie signed for a different name was accepted")
	}

	wrongType := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	wrongType.AddCookie(signed)

	var number int

	err = GetCookie(wrongType, "session", &number)
	if err == nil {
		t.Fatal("string cookie decoded into an int")
	}
}

func TestSetCookieRejectsUnencodableValue(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	err := SetCookie(recorder, request, "session", make(chan struct{}), "lax", "/")
	if err == nil {
		t.Fatal("SetCookie accepted a channel")
	}

	if headers := recorder.Header().Values("Set-Cookie"); len(headers) != 0 {
		t.Fatalf("SetCookie emitted headers: %v", headers)
	}

	issued, err := CreateNewCookie("session", func() {})
	if err == nil || issued != nil {
		t.Fatalf("CreateNewCookie(func) = (%v, %v)", issued, err)
	}
}

func TestCookieConcurrentRoundTrip(t *testing.T) {
	const workers = 32

	var group sync.WaitGroup

	for worker := range workers {
		group.Go(func() {
			name := fmt.Sprintf("cookie%d", worker)
			value := fmt.Sprintf("value-%d", worker)

			signed, err := CreateNewCookie(name, value, "none", "/parallel")
			if err != nil {
				t.Errorf("CreateNewCookie(%s): %v", name, err)

				return
			}

			if signed.SameSite != http.SameSiteNoneMode || signed.Path != "/parallel" ||
				!signed.Secure {
				t.Errorf("cookie attributes = %#v", signed)
			}

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
			request.AddCookie(signed)

			var loaded string

			err = GetCookie(request, name, &loaded)
			if err != nil {
				t.Errorf("GetCookie(%s): %v", name, err)
			}

			if loaded != value {
				t.Errorf("loaded %q, want %q", loaded, value)
			}
		})
	}

	group.Wait()
}

func flipEncodedValue(value string) string {
	if value == "" {
		return "x"
	}

	index := len(value) / 2
	replacement := byte('A')

	if value[index] == replacement {
		replacement = 'B'
	}

	return value[:index] + string(replacement) + value[index+1:]
}
