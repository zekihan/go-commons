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

func CreateNewCookie(name string, value any, opts ...string) (*http.Cookie, error) {
	sameSite := http.SameSiteLaxMode
	path := "/"

	switch len(opts) {
	case 1:
		sameSiteStr := opts[0]
		switch sameSiteStr {
		case "lax":
			sameSite = http.SameSiteLaxMode
		case "strict":
			sameSite = http.SameSiteStrictMode
		case "none":
			sameSite = http.SameSiteNoneMode
		}
	case 2: //nolint:mnd // false positive
		sameSiteStr := opts[0]
		switch sameSiteStr {
		case "lax":
			sameSite = http.SameSiteLaxMode
		case "strict":
			sameSite = http.SameSiteStrictMode
		case "none":
			sameSite = http.SameSiteNoneMode
		}

		path = opts[1]
	}

	encode, err := getSecureCookie().Encode(name, value)
	if err != nil {
		return nil, fmt.Errorf("failed to encode cookie value: %w", err)
	}

	newCookie := &http.Cookie{
		Name:     name,
		Value:    encode,
		Path:     path,
		MaxAge:   int(defaultTimeout),
		Secure:   true,
		HttpOnly: true,
		SameSite: sameSite,
	}

	return newCookie, nil
}

func InitSecureCookie(hashKey, blockKey []byte) {
	scOnce.Do(func() {
		sc = securecookie.New(hashKey, blockKey)
	})
}

func getSecureCookie() *securecookie.SecureCookie {
	if sc == nil {
		hashKey := securecookie.GenerateRandomKey(HashKeyLength)
		blockKey := securecookie.GenerateRandomKey(BlockKeyLength)
		InitSecureCookie(hashKey, blockKey)

		if sc == nil {
			panic("SecureCookie initialization failed")
		}
	}

	return sc
}

func SetCookie(w http.ResponseWriter, _ *http.Request, name string, value any, opts ...string) {
	c, err := CreateNewCookie(name, value, opts...)
	if err != nil {
		return
	}

	http.SetCookie(w, c)
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
