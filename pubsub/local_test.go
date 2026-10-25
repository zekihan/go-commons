package pubsub

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
)

func TestLocalPubSub_PublishAndListen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ps := NewLocalPubSub(&UniversalPubSubOptions{Prefix: "test"})

		const listeners = 3

		received := make([]chan string, listeners)

		cancels := make([]context.CancelFunc, listeners)
		for index := range listeners {
			messages := make(chan string, 1)
			received[index] = messages

			cancel := ps.Listen("channel", func(_ context.Context, payload string) error {
				messages <- payload

				return nil
			})
			cancels[index] = cancel
		}

		defer func() {
			for _, cancel := range cancels {
				cancel()
			}
		}()

		for _, message := range []string{"first", "second"} {
			err := ps.Publish(t.Context(), "channel", message)
			if err != nil {
				t.Fatalf("Publish failed: %v", err)
			}

			for index, messages := range received {
				if payload := <-messages; payload != message {
					t.Errorf("subscriber %d: expected %q, got %q", index, message, payload)
				}
			}
		}

		cancels[0]()

		err := ps.Publish(t.Context(), "channel", "remaining")
		if err != nil {
			t.Fatalf("Publish after canceling one subscriber failed: %v", err)
		}

		for _, messages := range received[1:] {
			if payload := <-messages; payload != "remaining" {
				t.Errorf("remaining subscriber received %q", payload)
			}
		}
	})
}

func TestLocalPubSub_CancelAndRelisten(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Also exercise the usable zero value.
		ps := &LocalPubSub{}
		cancel := ps.Listen("channel", func(_ context.Context, _ string) error {
			t.Error("canceled listener received a message")

			return nil
		})
		cancel()

		received := make(chan string, 1)

		nextCancel := ps.Listen("channel", func(_ context.Context, payload string) error {
			received <- payload

			return nil
		})
		defer nextCancel()

		// Repeated cancellation must not remove the replacement subscription.
		cancel()

		err := ps.Publish(t.Context(), "channel", "replacement")
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}

		if payload := <-received; payload != "replacement" {
			t.Errorf("unexpected payload %q", payload)
		}

		nextCancel()

		if subscribers := ps.getSubscribers("channel"); len(subscribers) != 0 {
			t.Errorf("expected no remaining subscribers, got %d", len(subscribers))
		}

		err = ps.Publish(t.Context(), "channel", "discarded")
		if err != nil {
			t.Fatalf("Publish with no subscribers failed: %v", err)
		}
	})
}

func TestLocalPubSub_BlockedPublishCancellation(t *testing.T) {
	for _, cancelPublisher := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "subscriber", true: "publisher"}[cancelPublisher],
			func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					ps := NewLocalPubSub(nil)
					entered := make(chan struct{})
					release := make(chan struct{})

					cancel := ps.Listen("channel", func(ctx context.Context, _ string) error {
						close(entered)
						<-release

						if !cancelPublisher && ctx.Err() == nil {
							t.Error("handler context was not canceled")
						}

						return nil
					})
					defer cancel()

					err := ps.Publish(t.Context(), "channel", "first")
					if err != nil {
						t.Fatalf("initial Publish failed: %v", err)
					}

					<-entered

					ctx, stopPublish := context.WithCancel(t.Context())
					defer stopPublish()

					result := make(chan error, 1)

					go func() { result <- ps.Publish(ctx, "channel", "blocked") }()

					synctest.Wait()

					select {
					case earlyErr := <-result:
						t.Fatalf("Publish did not block: %v", earlyErr)
					default:
					}

					if cancelPublisher {
						stopPublish()
					} else {
						cancel()
					}

					err = <-result
					if cancelPublisher && !errors.Is(err, context.Canceled) {
						t.Errorf("expected context cancellation, got %v", err)
					}

					if !cancelPublisher && err != nil {
						t.Errorf("subscriber cancellation failed Publish: %v", err)
					}

					close(release)
				})
			},
		)
	}
}

func TestLocalPubSub_SimultaneousCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ps := NewLocalPubSub(nil)
		start := make(chan struct{})

		var workers sync.WaitGroup

		for range 16 {
			cancel := ps.Listen("channel", func(_ context.Context, _ string) error { return nil })

			workers.Go(func() {
				<-start
				cancel()
				cancel()
			})
		}

		workers.Go(func() {
			<-start

			for range 16 {
				err := ps.Publish(t.Context(), "channel", "message")
				if err != nil {
					t.Errorf("Publish failed: %v", err)
				}
			}
		})
		synctest.Wait()
		close(start)
		workers.Wait()

		if subscribers := ps.getSubscribers("channel"); len(subscribers) != 0 {
			t.Errorf("expected no remaining subscribers, got %d", len(subscribers))
		}
	})
}

func TestLocalPubSub_CanceledPublish(t *testing.T) {
	ps := NewLocalPubSub(nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := ps.Publish(ctx, "channel", "message")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context cancellation, got %v", err)
	}
}
