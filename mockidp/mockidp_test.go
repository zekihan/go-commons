package mockidp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStartServesDiscovery(t *testing.T) {
	t.Parallel()

	server, err := Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = server.Close() })

	if server.Config.Issuer == "" || server.Config.ClientID == "" ||
		server.Config.ClientSecret == "" || server.Config.Subject == "" {
		t.Fatalf("incomplete config: %+v", server.Config)
	}

	if server.Provider() == nil {
		t.Fatal("Provider is nil")
	}

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		server.Config.Issuer+"/.well-known/openid-configuration",
		http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("discovery status = %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(body), `"issuer"`) {
		t.Fatalf("discovery body = %s", body)
	}
}

func TestRunWritesConfigAndStops(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())

	buf := &syncBuffer{}

	done := make(chan error, 1)

	go func() {
		done <- Run(ctx, buf)
	}()

	// Wait for the one-line JSON config.
	var cfg Config

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if json.Unmarshal(bytes.TrimSpace(buf.bytes()), &cfg) == nil && cfg.Issuer != "" {
			break
		}

		runtime.Gosched()
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}

	if cfg.Issuer == "" || cfg.Subject == "" {
		t.Fatalf("run config = %+v", cfg)
	}
}

type syncBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p) //nolint:wrapcheck // bytes.Buffer writes do not fail.
}

func (b *syncBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	return bytes.Clone(b.buf.Bytes())
}
