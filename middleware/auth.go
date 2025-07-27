package middleware

import (
	"net/http"
)

func GetAuthMiddlewareForSingleAPIKey(apiKey string) func(http.Handler) http.Handler {
	if apiKey == "" {
		return func(handler http.Handler) http.Handler {
			return handler
		}
	}

	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userAPIKey := r.Header.Get("X-Api-Key")
			if userAPIKey == "" {
				userAPIKey = r.URL.Query().Get("apiKey")
			}

			if userAPIKey != apiKey {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)

				return
			}

			handler.ServeHTTP(w, r)
		})
	}
}
