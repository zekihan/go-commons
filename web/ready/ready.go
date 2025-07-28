package ready

import (
	"net/http"

	"github.com/zekihan/go-commons/web"
)

func GetReady(w http.ResponseWriter, _ *http.Request) {
	web.WriteString(w, "OK\n")
}
