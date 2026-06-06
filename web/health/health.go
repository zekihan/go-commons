package health

import (
	"net/http"

	"github.com/zekihan/go-commons/web"
)

func GetHealth(w http.ResponseWriter, r *http.Request) {
	web.WriteString(r.Context(), w, "OK\n")
}
