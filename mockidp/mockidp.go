// Package mockidp provides a reusable loopback-only OpenID provider for
// end-to-end and integration tests, backed by mockoidc.
//
// Start serves a provider on a random loopback port for Go tests. Run does
// the same for external test harnesses: it writes the provider configuration
// as one JSON line and blocks until the context ends, so a small command can
// expose it to browser tests that cannot import Go code.
package mockidp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/oauth2-proxy/mockoidc"
)

// Config is the provider configuration a test harness needs to point an
// application at the mock provider.
//

type Config struct {
	Issuer       string `json:"issuer"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Subject      string `json:"subject"`
}

// Server is a running mock provider.
type Server struct {
	provider *mockoidc.MockOIDC
	Config   Config
}

// Start creates a provider listening on a random loopback port.
func Start(ctx context.Context) (*Server, error) {
	provider, err := mockoidc.NewServer(nil)
	if err != nil {
		return nil, fmt.Errorf("create provider: %w", err)
	}

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on loopback: %w", err)
	}

	err = provider.Start(listener, nil)
	if err != nil {
		_ = listener.Close()

		return nil, fmt.Errorf("start provider: %w", err)
	}

	return &Server{
		provider: provider,
		Config: Config{
			Issuer:       provider.Issuer(),
			ClientID:     provider.ClientID,
			ClientSecret: provider.ClientSecret,
			Subject:      mockoidc.DefaultUser().Subject,
		},
	}, nil
}

// Provider returns the underlying mock provider, for queuing test users or
// adding middleware in Go tests.
func (s *Server) Provider() *mockoidc.MockOIDC {
	return s.provider
}

// Close shuts the provider down.
func (s *Server) Close() error {
	err := s.provider.Shutdown()
	if err != nil {
		return fmt.Errorf("shutdown mock provider: %w", err)
	}

	return nil
}

// Run starts a provider, writes its Config as one JSON line to out, and
// blocks until ctx is done. It is the reusable core of the small mock-IdP
// commands external (non-Go) e2e harnesses launch.
func Run(ctx context.Context, out io.Writer) error {
	server, err := Start(ctx)
	if err != nil {
		return err
	}

	//nolint:gosec // Test-only provider; the generated secret is the harness config.
	err = json.NewEncoder(out).Encode(server.Config)
	if err != nil {
		_ = server.Close()

		return fmt.Errorf("print provider configuration: %w", err)
	}

	<-ctx.Done()

	return server.Close()
}
