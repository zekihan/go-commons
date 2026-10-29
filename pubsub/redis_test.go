package pubsub

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/redis/go-redis/v9"
)

func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	mr := miniredis.RunT(t)

	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestNewRedisPubSub_NilRedis(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil Redis client")
		}
	}()

	NewRedisPubSub(&UniversalPubSubOptions{})
}

func TestNewRedisPubSub_Valid(t *testing.T) {
	client := newTestRedisClient(t)

	ps := NewRedisPubSub(&UniversalPubSubOptions{
		Redis:  client,
		Prefix: "test",
	})

	if ps == nil {
		t.Fatal("expected non-nil RedisPubSub")
	}
}

func TestRedisPubSub_Publish_Success(t *testing.T) {
	client := newTestRedisClient(t)
	ps := NewRedisPubSub(&UniversalPubSubOptions{Redis: client})

	err := ps.Publish(t.Context(), "test-channel", "hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRedisPubSub_Listen_ReceivesMessage(t *testing.T) {
	client := newTestRedisClient(t)
	ps := NewRedisPubSub(&UniversalPubSubOptions{Redis: client})

	var wg sync.WaitGroup
	wg.Add(1)

	received := ""

	startup, stop := context.WithTimeout(t.Context(), 2*time.Second)
	defer stop()

	cancel, listenErr := ps.ListenReady(
		startup,
		"test-channel",
		func(ctx context.Context, payload string) error {
			received = payload

			wg.Done()

			return nil
		},
	)
	if listenErr != nil {
		t.Fatal(listenErr)
	}
	defer cancel()

	err := ps.Publish(t.Context(), "test-channel", "hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	waitCh := make(chan struct{})

	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		if received != "hello" {
			t.Errorf("expected message %q, got %q", "hello", received)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestRedisPubSub_Listen_WithPrefix(t *testing.T) {
	client := newTestRedisClient(t)
	ps := NewRedisPubSub(&UniversalPubSubOptions{Redis: client, Prefix: "myapp"})

	var wg sync.WaitGroup
	wg.Add(1)

	received := ""

	startup, stop := context.WithTimeout(t.Context(), 2*time.Second)
	defer stop()

	cancel, listenErr := ps.ListenReady(
		startup,
		"events",
		func(ctx context.Context, payload string) error {
			received = payload

			wg.Done()

			return nil
		},
	)
	if listenErr != nil {
		t.Fatal(listenErr)
	}
	defer cancel()

	err := ps.Publish(t.Context(), "events", "ping")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	waitCh := make(chan struct{})

	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		if received != "ping" {
			t.Errorf("expected message %q, got %q", "ping", received)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestRedisPubSub_Listen_Cancel(t *testing.T) {
	client := newTestRedisClient(t)
	ps := NewRedisPubSub(&UniversalPubSubOptions{Redis: client})

	ctx, stop := context.WithTimeout(t.Context(), 2*time.Second)
	defer stop()

	cancel, err := ps.ListenReady(ctx, "test-channel", func(context.Context, string) error {
		t.Error("handler called after cancellation")

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	cancel()
	cancel()

	err = ps.Publish(t.Context(), "test-channel", "msg")
	if err != nil {
		t.Fatal(err)
	}
}

func TestRedisPubSub_DelayedReadiness(t *testing.T) {
	mr := miniredis.RunT(t)
	entered, release := make(chan struct{}), make(chan struct{})

	mr.Server().SetPreHook(func(_ *server.Peer, cmd string, _ ...string) bool {
		if cmd == "SUBSCRIBE" {
			close(entered)
			<-release
		}

		return false
	})
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Cleanup(func() { _ = client.Close() })

	ps := NewRedisPubSub(&UniversalPubSubOptions{Redis: client, Prefix: "prefix"})

	ctx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()

	type result struct {
		cancel context.CancelFunc
		err    error
	}

	ready := make(chan result, 1)
	messages := make(chan string, 1)

	go func() {
		cancel, err := ps.ListenReady(
			ctx,
			"events",
			func(_ context.Context, msg string) error {
				messages <- msg

				return nil
			},
		)
		ready <- result{cancel, err}
	}()

	<-entered

	select {
	case r := <-ready:
		t.Fatalf("ready before acknowledgement: %v", r.err)
	default:
	}

	close(release)

	r := <-ready
	if r.err != nil {
		t.Fatal(r.err)
	}
	defer r.cancel()

	err := ps.Publish(ctx, "events", "message")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-messages:
		if msg != "message" {
			t.Fatalf("got %q", msg)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestRedisPubSub_StartupFailure(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.SetError("ERR subscription rejected")
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})

	t.Cleanup(func() { _ = client.Close() })

	ps := NewRedisPubSub(&UniversalPubSubOptions{Redis: client})

	ctx, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()

	cancel, err := ps.ListenReady(ctx, "events", func(context.Context, string) error { return nil })
	if err == nil {
		cancel()
		t.Fatal("expected startup error")
	}
}

func TestRedisPubSub_StartupCancellation(t *testing.T) {
	mr := miniredis.RunT(t)
	entered, release := make(chan struct{}), make(chan struct{})

	mr.Server().SetPreHook(func(_ *server.Peer, cmd string, _ ...string) bool {
		if cmd == "SUBSCRIBE" {
			close(entered)
			<-release
		}

		return false
	})
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Cleanup(func() { _ = client.Close() })

	ps := NewRedisPubSub(&UniversalPubSubOptions{Redis: client})
	ctx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	result := make(chan error, 1)

	go func() {
		_, err := ps.ListenReady(ctx, "events", func(context.Context, string) error { return nil })
		result <- err
	}()

	<-entered
	stop()

	err := <-result

	close(release)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestRedisPubSub_StartupTimeout(t *testing.T) {
	mr := miniredis.RunT(t)
	entered, release := make(chan struct{}), make(chan struct{})

	mr.Server().SetPreHook(func(_ *server.Peer, cmd string, _ ...string) bool {
		if cmd == "SUBSCRIBE" {
			close(entered)
			<-release
		}

		return false
	})
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Cleanup(func() { _ = client.Close() })

	ps := NewRedisPubSub(&UniversalPubSubOptions{Redis: client})

	ctx, stop := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer stop()

	result := make(chan error, 1)

	go func() {
		_, err := ps.ListenReady(ctx, "events", func(context.Context, string) error { return nil })
		result <- err
	}()

	<-entered

	err := <-result

	close(release)

	var netErr net.Error
	if !errors.Is(err, context.DeadlineExceeded) &&
		(!errors.As(err, &netErr) || !netErr.Timeout()) {
		t.Fatalf("expected timeout, got %v", err)
	}
}
