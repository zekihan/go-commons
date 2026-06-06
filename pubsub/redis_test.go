package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
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

	cancel := ps.Listen("test-channel", func(ctx context.Context, payload string) error {
		received = payload

		wg.Done()

		return nil
	})
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

	cancel := ps.Listen("events", func(ctx context.Context, payload string) error {
		received = payload

		wg.Done()

		return nil
	})
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

	var called bool

	cancel := ps.Listen("test-channel", func(ctx context.Context, payload string) error {
		called = true

		return nil
	})

	cancel()

	// Busy-wait for the goroutine to process the cancellation signal.
	for i := range 100000000 {
		_ = i
	}

	err := ps.Publish(t.Context(), "test-channel", "msg")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if called {
		t.Error("handler should not be called after cancel")
	}
}
