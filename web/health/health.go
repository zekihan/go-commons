package health

import (
	"net/http"

	"github.com/zekihan/go-commons/web"
)

func GetHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	web.WriteString(r.Context(), w, "OK\n")
}
