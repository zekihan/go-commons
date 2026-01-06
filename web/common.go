package web

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/zekihan/go-commons/utils"
)

func Write(w http.ResponseWriter, b []byte) bool {
	_, err := w.Write(b)

	return CheckError(w, err, http.StatusInternalServerError, "failed to write string")
}

func WriteString(w http.ResponseWriter, s string) bool {
	_, err := io.WriteString(w, s)

	return CheckError(w, err, http.StatusInternalServerError, "failed to write string")
}

// CheckError checks the error, and writes to w if there is an error.  Returns
// true if err is non-nil.
func CheckError(w http.ResponseWriter, err error, statusCode int, messages ...string) bool {
	if err != nil {
		err = utils.WrapErrors(err, messages...)
		if err == nil {
			return false
		}

		if statusCode == 0 {
			statusCode = http.StatusInternalServerError
		}

		if statusCode >= http.StatusInternalServerError {
			slog.Error(
				"Returned non 2xx http code",
				slog.Int("httpCode", statusCode),
				slog.Any("error", err),
			)
		} else {
			slog.Warn("Returned non 2xx http code", slog.Any("error", err))
		}

		http.Error(w, err.Error(), statusCode)

		return true
	}

	return false
}
