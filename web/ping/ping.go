package ping

import (
	"net/http"

	"github.com/zekihan/go-commons/web"
)

func GetPing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	web.WriteString(r.Context(), w, "Pong\n")
}
