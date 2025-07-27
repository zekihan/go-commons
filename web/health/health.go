package health

import (
	"net/http"

	"github.com/zekihan/go-commons/web"
)

func GetHealth(w http.ResponseWriter, _ *http.Request) {
	web.WriteString(w, "OK\n")
}
