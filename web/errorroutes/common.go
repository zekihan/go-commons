package errorroutes

import (
	"net/http"
	"strconv"

	"github.com/zekihan/go-commons/web"
)

func SendErrorMessage(w http.ResponseWriter, r *http.Request, status int) {
	statusText := http.StatusText(status)
	statusTextVariant := strconv.Itoa(status) + " " + statusText

	switch web.GetAcceptType(r) {
	case web.AcceptTypeHTML:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		web.WriteString(w, "<html><body><h1>"+statusTextVariant+"</h1></body></html>")
	case web.AcceptTypeJSON:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		web.WriteString(w, `{"error": "`+statusText+`"}`)
	case web.AcceptTypePlain:
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		web.WriteString(w, statusText)
	case web.AcceptTypeNone:
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		web.WriteString(w, statusText)
	default:
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		web.WriteString(w, statusText)
	}
}
