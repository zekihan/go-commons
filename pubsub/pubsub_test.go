package pubsub

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

type dummyPubSub struct {
	publishCalled bool
	listenCalled  bool
}

func (d *dummyPubSub) Publish(_ context.Context, _, _ string) error {
	d.publishCalled = true

	return nil
}

func (d *dummyPubSub) Listen(
	_ string,
	_ func(ctx context.Context, payload string) error,
) context.CancelFunc {
	d.listenCalled = true

	return func() {}
}

func TestNewUniversalPubSub_WithRedis(t *testing.T) {
	mockRedis := &redis.Client{}

	ps := NewUniversalPubSub(&UniversalPubSubOptions{Redis: mockRedis})
	if ps.PubSuber == nil {
		t.Fatal("expected PubSuber to be set")
	}
}

func TestNewUniversalPubSub_WithoutRedis(t *testing.T) {
	ps := NewUniversalPubSub(&UniversalPubSubOptions{})
	if ps.PubSuber == nil {
		t.Fatal("expected PubSuber to be set")
	}
}

func TestUniversalPubSub_Publish_And_Listen(t *testing.T) {
	dummy := &dummyPubSub{}
	pubsub := &UniversalPubSub{PubSuber: dummy}

	err := pubsub.Publish(t.Context(), "chan", "msg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !dummy.publishCalled {
		t.Error("expected Publish to be called")
	}

	pubsub.Listen("chan", func(ctx context.Context, payload string) error { return nil })

	if !dummy.listenCalled {
		t.Error("expected Listen to be called")
	}
}
