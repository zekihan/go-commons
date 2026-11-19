package ready

import (
	"net/http"

	"github.com/zekihan/go-commons/web"
)

func GetReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	web.WriteString(r.Context(), w, "OK\n")
}
