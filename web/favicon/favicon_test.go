package favicon

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestGetFavicon(t *testing.T) {
	// Create a temporary file to act as the favicon
	tempDir := t.TempDir()

	tmpFile, err := os.CreateTemp(tempDir, "favicon.ico")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	content := []byte{0x00, 0x01, 0x02, 0x03}

	_, err = tmpFile.Write(content)
	if err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}

	err = tmpFile.Close()
	if err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	handler := GetFavicon(tmpFile.Name())
	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", http.NoBody)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	defer func(Body io.ReadCloser) {
		bodyCloseErr := Body.Close()
		if bodyCloseErr != nil {
			t.Errorf("failed to close response body: %v", bodyCloseErr)
		}
	}(resp.Body)

	// Check Cache-Control header
	cacheControl := resp.Header.Get("Cache-Control")
	if cacheControl != "public, max-age=31536000" {
		t.Errorf("unexpected Cache-Control header: got %q", cacheControl)
	}

	// Check file content
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if !bytes.Equal(body, content) {
		t.Errorf("unexpected body: got %v, want %v", body, content)
	}
}
