package ping

import (
	"net/http"

	"github.com/zekihan/go-commons/web"
)

func GetPing(w http.ResponseWriter, _ *http.Request) {
	web.WriteString(w, "Pong\n")
}
