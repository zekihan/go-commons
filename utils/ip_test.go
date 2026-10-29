package utils

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newReq(ctx context.Context, hdrs map[string]string, remoteAddr string) *http.Request {
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", http.NoBody)

	r.RemoteAddr = remoteAddr
	for k, v := range hdrs {
		r.Header.Set(k, v)
	}

	return r
}

func TestGetIP_XForwardedFor(t *testing.T) {
	req := newReq(
		t.Context(),
		map[string]string{"X-Forwarded-For": "203.0.113.1, 198.51.100.5"},
		"",
	)

	ip, err := GetIP(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ip != "203.0.113.1" {
		t.Errorf("got %q, want %q", ip, "203.0.113.1")
	}
}

func TestGetIP_XRealIP(t *testing.T) {
	req := newReq(t.Context(), map[string]string{"X-Real-IP": "198.51.100.7"}, "")

	ip, err := GetIP(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ip != "198.51.100.7" {
		t.Errorf("got %q, want %q", ip, "198.51.100.7")
	}
}

func TestGetIP_RemoteAddr_IPv4(t *testing.T) {
	req := newReq(t.Context(), nil, "192.0.2.5:1234")

	ip, err := GetIP(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ip != "192.0.2.5" {
		t.Errorf("got %q, want %q", ip, "192.0.2.5")
	}
}

func TestGetIP_RemoteAddr_IPv6Loopback(t *testing.T) {
	req := newReq(t.Context(), nil, "[::1]:8080")

	ip, err := GetIP(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ip != "127.0.0.1" {
		t.Errorf("got %q, want %q", ip, "127.0.0.1")
	}
}

func TestGetIP_RemoteAddr_IPv6Zone(t *testing.T) {
	t.Parallel()

	for _, zone := range []string{"eth0", "3"} {
		t.Run(zone, func(t *testing.T) {
			t.Parallel()

			remote := &net.TCPAddr{
				IP:   net.ParseIP("fe80::1234"),
				Port: 8080,
				Zone: zone,
			}
			req := newReq(t.Context(), nil, remote.String())

			ip, err := GetIP(req)
			if err != nil {
				t.Fatalf("GetIP(%q): %v", remote, err)
			}

			if ip != "fe80::1234" {
				t.Errorf("got %q, want fe80::1234", ip)
			}
		})
	}
}

func TestGetIP_InvalidRemoteAddr_NoPort(t *testing.T) {
	// No headers and bad RemoteAddr.
	req := newReq(t.Context(), nil, "badaddr")

	_, err := GetIP(req)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	// Should at least include errIPNotFound.
	if !errors.Is(err, ErrIPNotFound) {
		t.Errorf("expected error to wrap %v, got %v", ErrIPNotFound, err)
	}
}

func TestGetIP_InvalidRemoteAddrWithPort(t *testing.T) {
	// No headers and invalid RemoteAddr with port.
	req := newReq(t.Context(), nil, "invalidaddr:1234")

	_, err := GetIP(req)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	// Should at least include errIPNotFound.
	if !errors.Is(err, ErrIPNotFound) {
		t.Errorf("expected error to wrap %v, got %v", ErrIPNotFound, err)
	}
}
