package middleware

import (
	"log/slog"
	"net/http"
)

func RecoverHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				//nolint:err113,errorlint // net/http recognizes the exact panic value.
				if err == http.ErrAbortHandler {
					panic(err)
				}

				slog.ErrorContext(
					r.Context(),
					"Panic recovered in HTTP handler",
					slog.Any("error", err),
				)
				http.Error(
					w,
					http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError,
				)
			}
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
