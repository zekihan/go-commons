package middleware

import (
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/zekihan/go-commons/cookie"
	"github.com/zekihan/go-commons/session"
	"github.com/zekihan/go-commons/web"
)

// GetAuthMiddlewareForCookie requires a valid session when enabled.
// An empty cookieKey rejects all requests; only enabled=false bypasses authentication.
func GetAuthMiddlewareForCookie(enabled bool, cookieKey string) func(http.Handler) http.Handler {
	if !enabled {
		return func(handler http.Handler) http.Handler {
			return handler
		}
	}

	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionCookie, ok := getCookie(r, cookieKey)
			if !ok || !sessionCookie.Authenticated || sessionCookie.IsExpired() {
				redirect(w, r)

				return
			}

			handler.ServeHTTP(w, r)
		})
	}
}

func getCookie(r *http.Request, cookieKey string) (*session.Cookie, bool) {
	// An enabled middleware must never accept an unconfigured cookie name.
	if cookieKey == "" {
		return nil, false
	}

	var sessionCookie session.Cookie

	err := cookie.GetCookie(r, cookieKey, &sessionCookie)
	if err != nil {
		return nil, false
	}

	return &sessionCookie, true
}

func redirect(w http.ResponseWriter, r *http.Request) {
	currentPath := r.URL.RequestURI()

	destination := "/login"
	if isLocalRequestURI(currentPath) {
		destination += "?" + url.Values{"redirect": {currentPath}}.Encode()
	}

	isHx := r.Header.Get("Hx-Request") == "true"

	if !isHx {
		http.Redirect(w, r, destination, http.StatusSeeOther)
	} else {
		w.Header().Set("Hx-Refresh", "true")
		w.WriteHeader(http.StatusUnauthorized)
		web.WriteString(r.Context(), w, "Unauthorized")
	}
}

// isLocalRequestURI excludes destinations browsers may interpret as another origin.
func isLocalRequestURI(target string) bool {
	parsed, err := url.ParseRequestURI(target)
	if err != nil {
		return false
	}

	return !parsed.IsAbs() && parsed.Host == "" &&
		strings.HasPrefix(parsed.Path, "/") && !strings.HasPrefix(parsed.Path, "//") &&
		!strings.Contains(parsed.Path, `\`) && !strings.ContainsFunc(parsed.Path, unicode.IsControl)
}
