package cookie

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/securecookie"
)

var (
	sc             *securecookie.SecureCookie
	scOnce         sync.Once
	defaultTimeout = 24 * time.Hour.Seconds()
)

const (
	HashKeyLength  = 64
	BlockKeyLength = 32
)

func parseSameSite(s string) http.SameSite {
	switch s {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	case "lax":
		return http.SameSiteLaxMode
	default:
		return http.SameSiteLaxMode
	}
}

// CreateNewCookie encodes value and validates the cookie's HTTP attributes.
// Invalid names or paths return an error rather than being silently dropped or
// altered when the cookie is serialized by net/http.
func CreateNewCookie(name string, value any, opts ...string) (*http.Cookie, error) {
	sameSite := http.SameSiteLaxMode
	path := "/"

	if len(opts) >= 1 {
		sameSite = parseSameSite(opts[0])
	}

	if len(opts) >= 2 { //nolint:mnd // false positive
		path = opts[1]
	}

	encode, err := getSecureCookie().Encode(name, value)
	if err != nil {
		return nil, fmt.Errorf("failed to encode cookie value: %w", err)
	}

	//nolint:gosec // generic method
	newCookie := &http.Cookie{
		Name:     name,
		Value:    encode,
		Path:     path,
		MaxAge:   int(defaultTimeout),
		Secure:   true,
		HttpOnly: true,
		SameSite: sameSite,
	}

	err = newCookie.Valid()
	if err != nil {
		return nil, fmt.Errorf("invalid cookie attributes: %w", err)
	}

	return newCookie, nil
}

// InitSecureCookie configures the process-wide codec on first use. Call it before
// using cookies to guarantee configured keys; otherwise lazy initialization may win.
func InitSecureCookie(hashKey, blockKey []byte) {
	scOnce.Do(func() {
		sc = securecookie.New(hashKey, blockKey)
	})
}

func getSecureCookie() *securecookie.SecureCookie {
	scOnce.Do(func() {
		hashKey := securecookie.GenerateRandomKey(HashKeyLength)
		blockKey := securecookie.GenerateRandomKey(BlockKeyLength)
		sc = securecookie.New(hashKey, blockKey)
	})

	return sc
}

func SetCookie(
	w http.ResponseWriter,
	_ *http.Request,
	name string,
	value any,
	opts ...string,
) error {
	c, err := CreateNewCookie(name, value, opts...)
	if err != nil {
		return err
	}

	http.SetCookie(w, c)

	return nil
}

func GetCookie[T any](r *http.Request, name string, value *T) error {
	c, err := r.Cookie(name)
	if err != nil {
		return fmt.Errorf("failed to get cookie %s: %w", name, err)
	}

	err = getSecureCookie().Decode(name, c.Value, value)
	if err != nil {
		return fmt.Errorf("failed to decode cookie %s: %w", name, err)
	}

	return nil
}
