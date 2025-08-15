package middleware

import (
	"net/http"
	"strings"
)

func GetAuthMiddlewareForSingleAPIKey(apiKey string) func(http.Handler) http.Handler {
	if apiKey == "" {
		return func(handler http.Handler) http.Handler {
			return handler
		}
	}

	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userAPIKey := getAPIKeyFromRequest(r)

			if userAPIKey != apiKey {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)

				return
			}

			handler.ServeHTTP(w, r)
		})
	}
}

func getAPIKeyFromRequest(r *http.Request) string {
	userAPIKey := r.Header.Get("X-Api-Key")
	userAPIKey = strings.TrimSpace(userAPIKey)
	if userAPIKey != "" {
		return userAPIKey
	}

	userAPIKey = r.URL.Query().Get("apiKey")
	userAPIKey = strings.TrimSpace(userAPIKey)
	if userAPIKey != "" {
		return userAPIKey
	}

	_, password, ok := r.BasicAuth()
	if ok {
		password = strings.TrimSpace(password)
		if password != "" {
			return password
		}
	}

	return userAPIKey
}
