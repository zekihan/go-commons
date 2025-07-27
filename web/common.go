package web

import (
	"io"
	"net/http"

	log "github.com/sirupsen/logrus"

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

		if statusCode == 0 {
			statusCode = http.StatusInternalServerError
		}

		if statusCode >= http.StatusInternalServerError {
			log.Error(err)
		} else {
			log.Warn(err)
		}

		http.Error(w, err.Error(), statusCode)

		return true
	}

	return false
}
