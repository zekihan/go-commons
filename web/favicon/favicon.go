// Package favicon serves favicon files over HTTP.
//
// Deprecated: web/favicon has no known consumers and will be removed in a
// future major release.
package favicon

import (
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
)

func GetFavicon(path string) func(w http.ResponseWriter, r *http.Request) {
	dir, name := filepath.Split(path)
	if name == "" {
		name = "."
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		http.ServeFileFS(w, r, faviconDir(dir), name)
	}
}

type faviconDir http.Dir

func (dir faviconDir) Open(name string) (fs.File, error) {
	file, err := http.Dir(dir).Open(name)
	if err != nil {
		return nil, fmt.Errorf("open favicon: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()

		return nil, fmt.Errorf("stat favicon: %w", err)
	}

	if info.IsDir() {
		_ = file.Close()

		return nil, fs.ErrNotExist
	}

	return file, nil
}
