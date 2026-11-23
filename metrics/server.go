package metrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const shutdownTimeout = 5 * time.Second

// Server returns an HTTP server serving only Handler at /metrics on addr.
// Serve metrics on a separate, internal-only address (for example :9090)
// so /metrics is not exposed through public ingress:
//
//	go registry.ListenAndServe(ctx, ":9090")
func (registry *Registry) Server(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", registry.Handler())

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: shutdownTimeout,
	}
}

// ListenAndServe runs Server(addr) until ctx is canceled or the server
// fails. A graceful shutdown caused by ctx cancellation returns nil.
func (registry *Registry) ListenAndServe(ctx context.Context, addr string) error {
	server := registry.Server(addr)

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()

		_ = server.Shutdown(shutdownCtx) //nolint:errcheck // Best-effort drain.
	}()

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return fmt.Errorf("metrics: serve on %s: %w", addr, err)
}
