package session

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zekihan/go-commons/cookie"
)

func TestNewCookieReturnsNilWhenEntropyFails(t *testing.T) {
	previous := slog.Default()

	var logs bytes.Buffer

	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
		uuid.SetRand(nil)
	})

	uuid.DisableRandPool()
	uuid.SetRand(brokenEntropy{})

	if got := NewCookie(); got != nil {
		t.Fatalf("NewCookie() = %#v, want nil when entropy is unavailable", got)
	}

	if !bytes.Contains(logs.Bytes(), []byte("Error generating UUID for new cookie")) {
		t.Fatalf("entropy failure was not logged: %s", logs.String())
	}

	uuid.SetRand(nil)

	recovered := NewCookie()
	if recovered == nil || recovered.ID == "" || recovered.IsExpired() || recovered.Authenticated {
		t.Fatalf("NewCookie() after restoring entropy = %#v", recovered)
	}
}

func TestNewCookieConcurrent(t *testing.T) {
	const workers = 32

	ids := make([]string, workers)

	var group sync.WaitGroup

	for worker := range workers {
		group.Go(func() {
			created := NewCookie()
			if created == nil {
				t.Error("NewCookie() = nil")

				return
			}

			if created.Authenticated || created.IsExpired() || created.ID == "" {
				t.Errorf("NewCookie() = %#v", created)
			}

			if created.ExpireTime.Sub(created.Created) != defaultSessionTimeout {
				t.Errorf(
					"lifetime = %s, want %s",
					created.ExpireTime.Sub(created.Created),
					defaultSessionTimeout,
				)
			}

			parsed, err := uuid.Parse(created.ID)
			if err != nil || parsed.Version() != 7 {
				t.Errorf("session ID %q is not a version 7 UUID", created.ID)
			}

			ids[worker] = created.ID
		})
	}

	group.Wait()

	seen := make(map[string]struct{}, workers)

	for _, id := range ids {
		if id == "" {
			t.Fatal("missing session ID from a worker")
		}

		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate session ID %s", id)
		}

		seen[id] = struct{}{}
	}
}

func TestZeroSessionIsExpired(t *testing.T) {
	if !(&Cookie{}).IsExpired() {
		t.Fatal("zero-value session is usable")
	}
}

func TestSessionCookieRoundTripAndExpiry(t *testing.T) {
	issued := NewCookie()
	if issued == nil {
		t.Fatal("NewCookie() = nil")
	}

	issued.Authenticated = true

	var loaded Cookie

	roundTripSession(t, "session", issued, &loaded)

	if loaded.ID != issued.ID || loaded.Authenticated != issued.Authenticated ||
		!loaded.Created.Equal(issued.Created) || !loaded.ExpireTime.Equal(issued.ExpireTime) {
		t.Fatalf("lookup = %#v, want %#v", loaded, *issued)
	}

	if loaded.IsExpired() {
		t.Fatal("fresh session was expired after lookup")
	}

	issued.ExpireTime = time.Now().Add(-time.Minute)
	roundTripSession(t, "session", issued, &loaded)

	if !loaded.Authenticated || !loaded.IsExpired() || !loaded.ExpireTime.Equal(issued.ExpireTime) {
		t.Fatalf("expired session after lookup = %#v", loaded)
	}
}

func TestSessionLookupRejectsMissingAndTamperedCookies(t *testing.T) {
	issued := NewCookie()
	if issued == nil {
		t.Fatal("NewCookie() = nil")
	}

	issued.Authenticated = true

	signed, err := cookie.CreateNewCookie("session", issued)
	if err != nil {
		t.Fatalf("CreateNewCookie: %v", err)
	}

	missingRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	var loaded Cookie

	err = cookie.GetCookie(missingRequest, "session", &loaded)
	if !errors.Is(err, http.ErrNoCookie) {
		t.Fatalf("missing cookie error = %v, want ErrNoCookie", err)
	}

	tamperedRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	tamperedRequest.AddCookie(&http.Cookie{
		Name:     signed.Name,
		Value:    flipCookieValue(signed.Value),
		Path:     signed.Path,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	loaded = Cookie{ID: "sentinel", Authenticated: true}

	err = cookie.GetCookie(tamperedRequest, "session", &loaded)
	if err == nil || errors.Is(err, http.ErrNoCookie) {
		t.Fatalf("tampered cookie error = %v", err)
	}

	if loaded.ID != "sentinel" || !loaded.Authenticated {
		t.Fatalf("tampered cookie changed the destination: %#v", loaded)
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

	err = cookie.GetCookie(renamedRequest, "other", &loaded)
	if err == nil {
		t.Fatal("cookie signed for another name was accepted")
	}
}

func roundTripSession(t *testing.T, name string, issued, loaded *Cookie) {
	t.Helper()

	signed, err := cookie.CreateNewCookie(name, issued)
	if err != nil {
		t.Fatalf("CreateNewCookie: %v", err)
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	request.AddCookie(signed)

	err = cookie.GetCookie(request, name, loaded)
	if err != nil {
		t.Fatalf("GetCookie: %v", err)
	}
}

func flipCookieValue(value string) string {
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

type brokenEntropy struct{}

var errEntropyUnavailable = errors.New("entropy unavailable")

func (brokenEntropy) Read([]byte) (int, error) {
	return 0, errEntropyUnavailable
}
