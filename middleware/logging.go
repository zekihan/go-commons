package middleware

import (
	"net/http"
	"time"

	"github.com/felixge/httpsnoop"

	log "github.com/sirupsen/logrus"

	"github.com/zekihan/go-commons/utils"
)

var defaultSkippedPaths = []string{
	"/ping",
	"/health",
	"/ready",
	"/api/ping",
	"/api/health",
	"/api/ready",
}

var defaultSkippedContains = []string{
	"favicon.ico",
}

func Logging(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if utils.Contains(defaultSkippedPaths, r.URL.Path) {
			handler.ServeHTTP(w, r)

			return
		}

		if utils.Contains(defaultSkippedContains, r.URL.Path) {
			handler.ServeHTTP(w, r)

			return
		}

		ip, err := utils.GetIP(r)
		if err != nil {
			log.Error("Error getting IP: ", err)

			ip = "unknown"
		}

		m := httpsnoop.CaptureMetrics(handler, w, r)
		log.Infof(
			"%s %s %d %s %s",
			r.Method,
			ip,
			m.Code,
			m.Duration.Round(time.Millisecond),
			r.URL.Path,
		)
	})
}
