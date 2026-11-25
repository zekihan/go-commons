package middleware

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"net/http"

	"github.com/felixge/httpsnoop"
)

func RecoverHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		written := false
		wrapped := httpsnoop.Wrap(w, httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(code int) {
					if code >= 200 || code == http.StatusSwitchingProtocols {
						written = true
					}

					next(code)
				}
			},
			Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return func(p []byte) (int, error) {
					written = true

					return next(p)
				}
			},
			ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
				return func(r io.Reader) (int64, error) {
					written = true

					return next(r)
				}
			},
			Flush: func(next httpsnoop.FlushFunc) httpsnoop.FlushFunc {
				return func() { written = true; next() }
			},
			FlushError: func(next httpsnoop.FlushErrorFunc) httpsnoop.FlushErrorFunc {
				return func() error {
					written = true

					return next()
				}
			},
			Hijack: func(next httpsnoop.HijackFunc) httpsnoop.HijackFunc {
				return func() (net.Conn, *bufio.ReadWriter, error) {
					conn, rw, err := next()
					if err == nil {
						written = true
					}

					return conn, rw, err
				}
			},
		})

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

				if written {
					panic(http.ErrAbortHandler)
				}

				http.Error(
					w,
					http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError,
				)
			}
		}()

		next.ServeHTTP(wrapped, r)
	}

	return http.HandlerFunc(fn)
}
