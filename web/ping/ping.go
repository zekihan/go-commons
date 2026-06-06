package ping

import (
	"net/http"

	"github.com/zekihan/go-commons/web"
)

func GetPing(w http.ResponseWriter, r *http.Request) {
	web.WriteString(r.Context(), w, "Pong\n")
}
