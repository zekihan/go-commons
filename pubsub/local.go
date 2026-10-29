package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

type localSubscriber struct {
	messages chan string
	done     <-chan struct{}
}

// LocalPubSub delivers each publication to every listener registered when Publish starts.
// Listeners handle messages serially; concurrent publications have no defined order.
// A slow handler applies backpressure to Publish until it receives the next message.
type LocalPubSub struct {
	subscribers map[string]map[*localSubscriber]struct{}
	prefix      string
	mu          sync.RWMutex
}

func NewLocalPubSub(opts *UniversalPubSubOptions) *LocalPubSub {
	if opts == nil {
		opts = &UniversalPubSubOptions{}
	}

	return &LocalPubSub{
		subscribers: make(map[string]map[*localSubscriber]struct{}),
		prefix:      opts.Prefix,
	}
}

// Publish waits for each subscriber to receive the message, not for handlers to finish.
// Canceled subscribers are skipped. Context cancellation can leave a partial delivery;
// messages are not retained when there are no subscribers.
func (ps *LocalPubSub) Publish(ctx context.Context, channel, message string) error {
	chKey := getChannelName(ps.prefix, channel)
	subscribers := ps.getSubscribers(chKey)

	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("publish to %s: %w", chKey, err)
	}

	if len(subscribers) == 0 {
		slog.WarnContext(ctx, "No subscribers for channel", slog.String("channel", chKey))
	}

	for _, subscriber := range subscribers {
		select {
		case <-ctx.Done():
			return fmt.Errorf("publish to %s: %w", chKey, ctx.Err())
		case <-subscriber.done:
		case subscriber.messages <- message:
		}
	}

	return nil
}

// Listen registers an independent subscriber before returning. Its cancel function is
// idempotent and synchronously unregisters only this subscriber. An in-flight handler
// may finish after cancellation; its context is canceled. Cancel never waits for it.
func (ps *LocalPubSub) Listen(
	channel string,
	handler func(ctx context.Context, payload string) error,
) context.CancelFunc {
	chKey := getChannelName(ps.prefix, channel)

	ctx, cancel := context.WithCancel(context.Background())
	subscriber := &localSubscriber{messages: make(chan string), done: ctx.Done()}
	ps.addSubscriber(chKey, subscriber)
	slog.InfoContext(ctx, "Subscribed to channel", slog.String("channel", chKey))

	go func() {
		for {
			select {
			case msg := <-subscriber.messages:
				if ctx.Err() != nil {
					return
				}

				slog.InfoContext(
					ctx,
					"Received message",
					slog.String("channel", chKey),
					slog.String("message", msg),
				)

				err := handler(ctx, msg)
				if err != nil {
					slog.ErrorContext(
						ctx,
						"Error handling message",
						slog.String("channel", chKey),
						slog.String("message", msg),
						slog.Any("error", err),
					)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return func() {
		ps.mu.Lock()
		defer ps.mu.Unlock()

		cancel()
		delete(ps.subscribers[chKey], subscriber)

		if len(ps.subscribers[chKey]) == 0 {
			delete(ps.subscribers, chKey)
		}
	}
}

// ListenReady registers the listener before returning. Ctx bounds initialization only.
//
//nolint:contextcheck // Initialization context intentionally does not control listener lifetime.
func (ps *LocalPubSub) ListenReady(ctx context.Context, channel string,
	handler func(context.Context, string) error,
) (context.CancelFunc, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, ErrDeadlineRequired
	}

	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("initialize listener: %w", err)
	}

	cancel := ps.Listen(
		channel,
		handler,
	)

	err = ctx.Err()
	if err != nil {
		cancel()

		return nil, fmt.Errorf("initialize listener: %w", err)
	}

	return cancel, nil
}

func (ps *LocalPubSub) addSubscriber(channel string, subscriber *localSubscriber) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.subscribers == nil {
		ps.subscribers = make(map[string]map[*localSubscriber]struct{})
	}

	if ps.subscribers[channel] == nil {
		ps.subscribers[channel] = make(map[*localSubscriber]struct{})
	}

	ps.subscribers[channel][subscriber] = struct{}{}
}

func (ps *LocalPubSub) getSubscribers(channel string) []*localSubscriber {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	subscribers := make([]*localSubscriber, 0, len(ps.subscribers[channel]))
	for subscriber := range ps.subscribers[channel] {
		subscribers = append(subscribers, subscriber)
	}

	return subscribers
}
