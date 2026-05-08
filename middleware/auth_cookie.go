package middleware

import (
	"net/http"

	"github.com/zekihan/go-commons/cookie"
	"github.com/zekihan/go-commons/session"
	"github.com/zekihan/go-commons/web"
)

func GetAuthMiddlewareForCookie(enabled bool, cookieKey string) func(http.Handler) http.Handler {
	if !enabled || cookieKey == "" {
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
	var sessionCookie session.Cookie

	err := cookie.GetCookie(r, cookieKey, &sessionCookie)
	if err != nil {
		return nil, false
	}

	return &sessionCookie, true
}

func redirect(w http.ResponseWriter, r *http.Request, opts ...string) {
	path := "/login"
	currentPath := r.URL.Path

	switch len(opts) {
	case 1:
		path = opts[0]
	case 2: //nolint:mnd // false positive
		path = opts[0]
		currentPath = opts[1]
	}

	url := path
	if currentPath != "" {
		url += "?redirect=" + currentPath
	}

	isHx := r.Header.Get("Hx-Request") == "true"

	if !isHx {
		//nolint:gosec // generic method
		http.Redirect(w, r, url, http.StatusSeeOther)
	} else {
		w.Header().Set("Hx-Refresh", "true")
		w.WriteHeader(http.StatusUnauthorized)
		web.WriteString(w, "Unauthorized")
	}
}
