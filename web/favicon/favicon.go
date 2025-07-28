package favicon

import "net/http"

func GetFavicon(path string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		http.ServeFile(w, r, path)
	}
}
