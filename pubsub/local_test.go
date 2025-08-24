package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLocalPubSub_PublishAndListen(t *testing.T) {
	ps := NewLocalPubSub()
	channel := "test-channel"
	message := "hello"

	var wg sync.WaitGroup
	wg.Add(1)

	received := ""

	cancel := ps.Listen(channel, func(ctx context.Context, payload string) error {
		received = payload

		wg.Done()

		return nil
	})
	defer cancel()

	err := ps.Publish(t.Context(), channel, message)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	waitCh := make(chan struct{})

	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		if received != message {
			t.Errorf("Expected message %q, got %q", message, received)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

func TestLocalPubSub_CancelListen(t *testing.T) {
	ps := NewLocalPubSub()
	channel := "cancel-channel"

	var called bool

	cancel := ps.Listen(channel, func(ctx context.Context, payload string) error {
		called = true

		return nil
	})

	cancel()

	// wait for a sec. no sleep bust wait
	for i := range 100000000 {
		_ = i
	}

	err := ps.Publish(t.Context(), channel, "msg")
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if called {
		t.Error("Handler should not be called after cancel")
	}
}

func TestLocalPubSub_getOrCreateChannel(t *testing.T) {
	ps := &LocalPubSub{}

	ch1 := ps.getOrCreateChannel("foo")
	if ch1 == nil {
		t.Fatal("Expected non-nil channel")
	}

	ch2 := ps.getOrCreateChannel("foo")
	if ch1 != ch2 {
		t.Error("Expected same channel for same name")
	}

	ch3 := ps.getOrCreateChannel("bar")
	if ch3 == nil || ch3 == ch1 {
		t.Error("Expected different channel for different name")
	}
}

func TestLocalPubSub_getChannel(t *testing.T) {
	ps := &LocalPubSub{}

	ch1 := ps.getChannel("foo")
	if ch1 != nil {
		t.Fatal("Expected nil channel")
	}

	ch1 = ps.getOrCreateChannel("foo")

	ch2 := ps.getChannel("foo")
	if ch1 != ch2 {
		t.Error("Expected same channel for same name")
	}

	ch3 := ps.getChannel("bar")
	if ch3 != nil {
		t.Fatal("Expected nil channel")
	}

	ch3 = ps.getOrCreateChannel("bar")
	if ch3 == ch1 {
		t.Error("Expected different channel for different name")
	}
}
