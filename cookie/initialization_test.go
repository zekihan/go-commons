package cookie

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/securecookie"
)

func resetSecureCookie(t *testing.T) {
	t.Helper()
	// These tests are intentionally sequential. Reset only before starting workers
	// and after all workers have exited; never copy a used sync.Once.
	sc = nil
	scOnce = sync.Once{}

	t.Cleanup(func() {
		sc = nil
		scOnce = sync.Once{}
	})
}

func TestSecureCookieConcurrentInitialization(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "lazy", true: "explicit-versus-lazy"}[explicit],
			func(t *testing.T) {
				resetSecureCookie(t)

				const workers = 32

				start := make(chan struct{})
				codecs := make(chan *securecookie.SecureCookie, workers)

				var group sync.WaitGroup

				for range workers {
					group.Go(func() {
						<-start

						codecs <- getSecureCookie()

						assertCookieRoundTrip(t)
					})
				}

				if explicit {
					group.Go(func() {
						<-start
						InitSecureCookie(bytes.Repeat([]byte{1}, HashKeyLength),
							bytes.Repeat([]byte{2}, BlockKeyLength))
					})
				}

				close(start)
				group.Wait()
				close(codecs)

				codec := getSecureCookie()

				for got := range codecs {
					if got == nil || got != codec {
						t.Error("concurrent readers did not receive the same initialized codec")
					}
				}

				InitSecureCookie(bytes.Repeat([]byte{3}, HashKeyLength), nil)

				if getSecureCookie() != codec {
					t.Error("subsequent initialization replaced the winning codec")
				}
			},
		)
	}
}

func TestSecureCookieConfiguredKeys(t *testing.T) {
	resetSecureCookie(t)

	hashKey := bytes.Repeat([]byte{1}, HashKeyLength)
	blockKey := bytes.Repeat([]byte{2}, BlockKeyLength)
	InitSecureCookie(hashKey, blockKey)
	InitSecureCookie(bytes.Repeat([]byte{3}, HashKeyLength), nil)

	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			cookie, err := CreateNewCookie("configured", "value")
			if err != nil {
				t.Errorf("CreateNewCookie failed: %v", err)

				return
			}

			var value string

			err = securecookie.New(hashKey, blockKey).Decode("configured", cookie.Value, &value)
			if err != nil {
				t.Errorf("configured keys cannot decode cookie: %v", err)
			}

			if value != "value" {
				t.Errorf("unexpected decoded value %q", value)
			}
		})
	}

	group.Wait()
}

func assertCookieRoundTrip(t *testing.T) {
	t.Helper()

	cookie, err := CreateNewCookie("concurrent", "value")
	if err != nil {
		t.Errorf("CreateNewCookie failed: %v", err)

		return
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	request.AddCookie(cookie)

	var value string

	err = GetCookie(request, "concurrent", &value)
	if err != nil {
		t.Errorf("GetCookie failed: %v", err)
	}

	if value != "value" {
		t.Errorf("unexpected decoded value %q", value)
	}
}
