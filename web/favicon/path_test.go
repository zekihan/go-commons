package favicon

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGetFaviconContentTypeCacheAndHead(t *testing.T) {
	directory := t.TempDir()
	pngPath := filepath.Join(directory, "mark.png")
	content := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 'x'}

	err := os.WriteFile(pngPath, content, 0o600)
	if err != nil {
		t.Fatalf("write png: %v", err)
	}

	png := serveFavicon(t, pngPath, http.MethodGet, "/favicon.ico")
	if png.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", png.Code, http.StatusOK)
	}

	if png.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("Content-Type = %q", png.Header().Get("Content-Type"))
	}

	if png.Header().Get("Cache-Control") != "public, max-age=31536000" {
		t.Fatalf("Cache-Control = %q", png.Header().Get("Cache-Control"))
	}

	if !bytes.Equal(png.Body.Bytes(), content) {
		t.Fatalf("body = %v", png.Body.Bytes())
	}

	icoPath := filepath.Join(directory, "mark.ico")

	err = os.WriteFile(icoPath, content, 0o600)
	if err != nil {
		t.Fatalf("write ico: %v", err)
	}

	ico := serveFavicon(t, icoPath, http.MethodGet, "/favicon.ico")
	if ico.Code != http.StatusOK || ico.Header().Get("Content-Type") != "image/vnd.microsoft.icon" {
		t.Fatalf("ico status=%d Content-Type=%q", ico.Code, ico.Header().Get("Content-Type"))
	}

	head := serveFavicon(t, pngPath, http.MethodHead, "/favicon.ico")
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD status=%d body=%q", head.Code, head.Body.String())
	}

	if head.Header().Get("Content-Type") != "image/png" ||
		head.Header().Get("Cache-Control") != "public, max-age=31536000" ||
		head.Header().Get("Content-Length") != "9" {
		t.Fatalf("HEAD headers = %v", head.Header())
	}
}

func TestGetFaviconMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.ico")
	response := serveFavicon(t, path, http.MethodGet, "/favicon.ico")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}

	if response.Header().Get("Cache-Control") != "" {
		t.Fatalf("missing file was cacheable: %q", response.Header().Get("Cache-Control"))
	}
}

func TestGetFaviconRejectsDirectoryPathsAndRequestTraversal(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")

	err := os.Mkdir(child, 0o750)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	secret := []byte("top-secret-favicon")

	err = os.WriteFile(filepath.Join(parent, "secret.txt"), secret, 0o600)
	if err != nil {
		t.Fatalf("write secret: %v", err)
	}

	icon := []byte("icon-bytes")
	iconPath := filepath.Join(child, "mark.png")

	err = os.WriteFile(iconPath, icon, 0o600)
	if err != nil {
		t.Fatalf("write icon: %v", err)
	}

	err = os.WriteFile(filepath.Join(child, "index.html"), []byte("index-body"), 0o600)
	if err != nil {
		t.Fatalf("write index: %v", err)
	}

	cases := []struct {
		name   string
		path   string
		url    string
		status int
	}{
		{
			name:   "trailing separator",
			path:   child + string(os.PathSeparator),
			url:    "/favicon.ico",
			status: http.StatusNotFound,
		},
		{
			name:   "dotdot name",
			path:   child + "/..",
			url:    "/favicon.ico",
			status: http.StatusNotFound,
		},
		{
			name:   "request dotdot",
			path:   iconPath,
			url:    "/static/../secret.txt",
			status: http.StatusBadRequest,
		},
		{
			name:   "request nested dotdot",
			path:   iconPath,
			url:    "/a/../../secret.txt",
			status: http.StatusBadRequest,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			response := serveFavicon(t, tt.path, http.MethodGet, tt.url)
			body := response.Body.Bytes()

			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d; body %q", response.Code, tt.status, body)
			}

			if bytes.Contains(body, secret) || bytes.Contains(body, icon) ||
				bytes.Contains(body, []byte("index-body")) {
				t.Fatalf("handler leaked file bytes: %q", body)
			}

			if response.Header().Get("Cache-Control") == "public, max-age=31536000" {
				t.Fatal("rejected favicon was given the success cache policy")
			}
		})
	}

	relative := serveFavicon(t, iconPath, http.MethodGet, "/brand/mark.png")
	if relative.Code != http.StatusOK || !bytes.Equal(relative.Body.Bytes(), icon) {
		t.Fatalf(
			"configured file was not served: status=%d body=%q",
			relative.Code,
			relative.Body.String(),
		)
	}
}

func TestFaviconOpenErrors(t *testing.T) {
	directory := t.TempDir()
	root := faviconDir(directory)

	_, err := root.Open("missing.ico")
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file error = %v", err)
	}

	file, err := root.Open(".")
	if file != nil {
		_ = file.Close()
	}

	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("directory error = %v", err)
	}

	iconPath := filepath.Join(directory, "mark.png")

	err = os.WriteFile(iconPath, []byte("icon"), 0o600)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	file, err = root.Open("mark.png")
	if err != nil {
		t.Fatalf("open file: %v", err)
	}

	info, err := file.Stat()
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}

	if info.IsDir() || info.Name() != "mark.png" {
		t.Fatalf("file info = name %q dir %t", info.Name(), info.IsDir())
	}

	err = file.Close()
	if err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestGetFaviconEmptyPathDoesNotServeWorkingDirectoryFile(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)

	err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("index-body"), 0o600)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	response := serveFavicon(t, "", http.MethodGet, "/favicon.ico")
	if response.Code != http.StatusNotFound ||
		bytes.Contains(response.Body.Bytes(), []byte("index-body")) {
		t.Fatalf("empty path status=%d body=%q", response.Code, response.Body.String())
	}
}

func serveFavicon(t *testing.T, path, method, requestPath string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), method, requestPath, http.NoBody)
	recorder := httptest.NewRecorder()
	GetFavicon(path)(recorder, request)

	result := recorder.Result()
	defer func(body io.ReadCloser) {
		_ = body.Close()
	}(result.Body)

	return recorder
}
