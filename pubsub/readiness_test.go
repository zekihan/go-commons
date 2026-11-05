package pubsub

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestLocalListenReady(t *testing.T) {
	ps := NewLocalPubSub(nil)

	handler := func(context.Context, string) error { return nil }

	_, err := ps.ListenReady(t.Context(), "events", handler)
	if !errors.Is(err, ErrDeadlineRequired) {
		t.Fatalf("expected deadline error, got %v", err)
	}

	ctx, stop := context.WithTimeout(t.Context(), time.Second)
	stop()

	_, err = ps.ListenReady(ctx, "events", handler)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}

	ctx, stop = context.WithTimeout(t.Context(), time.Second)
	defer stop()

	cancel, err := ps.ListenReady(ctx, "events", handler)
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	if len(ps.getSubscribers("events")) != 0 {
		t.Fatal("listener remained registered")
	}
}

func TestUniversalListenReady(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Cleanup(func() { _ = client.Close() })

	for name, opts := range map[string]*UniversalPubSubOptions{"local": {}, "redis": {Redis: client}} {
		t.Run(name, func(t *testing.T) {
			ps := NewUniversalPubSub(opts)

			handler := func(context.Context, string) error { return nil }

			_, err := ps.ListenReady(t.Context(), "events", handler)
			if !errors.Is(err, ErrDeadlineRequired) {
				t.Fatalf("expected wrapped deadline error, got %v", err)
			}

			ctx, stop := context.WithTimeout(t.Context(), 2*time.Second)
			defer stop()

			cancel, err := ps.ListenReady(ctx, "events", handler)
			if err != nil {
				t.Fatal(err)
			}

			cancel()
		})
	}

	ps := &UniversalPubSub{PubSuber: &dummyPubSub{}}

	_, err := ps.ListenReady(t.Context(), "events", nil)
	if !errors.Is(err, ErrReadinessUnsupported) {
		t.Fatalf("expected unsupported error, got %v", err)
	}
}
