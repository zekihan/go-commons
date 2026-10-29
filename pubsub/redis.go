package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const legacyStartupTimeout = 5 * time.Second

type RedisPubSub struct {
	rdb    redis.UniversalClient
	prefix string
}

func NewRedisPubSub(opts *UniversalPubSubOptions) *RedisPubSub {
	if opts == nil {
		opts = &UniversalPubSubOptions{}
	}

	if opts.Redis == nil {
		panic("redis pubsub client is required")
	}

	return &RedisPubSub{
		rdb:    opts.Redis,
		prefix: opts.Prefix,
	}
}

func (ps *RedisPubSub) Publish(ctx context.Context, channel, message string) error {
	chKey := getChannelName(ps.prefix, channel)

	err := ps.rdb.Publish(ctx, chKey, message).Err()
	if err != nil {
		return fmt.Errorf("error publishing message to %s: %w", chKey, err)
	}

	slog.InfoContext(
		ctx,
		"Published message",
		slog.String("channel", chKey),
		slog.String("message", message),
	)

	return nil
}

func (ps *RedisPubSub) Listen(
	channel string,
	handler func(ctx context.Context, payload string) error,
) context.CancelFunc {
	// Legacy Listen starts asynchronously; use ListenReady to observe initialization.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		startup, stop := context.WithTimeout(ctx, legacyStartupTimeout)
		defer stop()

		closeListener, err := ps.ListenReady(startup, channel, handler)
		if err != nil {
			return
		}

		<-ctx.Done()
		closeListener()
	}()

	return cancel
}

// ListenReady waits for the server's subscription acknowledgement. The context bounds
// initialization only; call the returned cancel function to stop a successful listener.
// This guarantees initialization, not durable or exactly-once delivery.
//
//nolint:gocyclo // Startup and cancellation paths must each release the subscription.
func (ps *RedisPubSub) ListenReady(ctx context.Context, channel string,
	handler func(context.Context, string) error,
) (context.CancelFunc, error) {
	chKey := getChannelName(ps.prefix, channel)

	if _, ok := ctx.Deadline(); !ok {
		return nil, ErrDeadlineRequired
	}

	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("subscribe to %s: %w", chKey, err)
	}

	listenerCtx, cancel := context.WithCancel(context.Background())
	// Create the subscription before doing network I/O so cancellation can own cleanup.
	sub := ps.rdb.Subscribe(ctx)

	var once sync.Once

	closeSub := func() { once.Do(func() { cancel(); _ = sub.Close() }) }
	stop := context.AfterFunc(ctx, closeSub)

	// Subscribe can block in connection initialization even when ctx is canceled.
	// Keep that work off the caller; closeSub will release its connection once
	// go-redis releases the subscription lock.
	type startupResult struct {
		ack any
		err error
	}

	result := make(chan startupResult, 1)

	go func() {
		err := sub.Subscribe(ctx, chKey)
		if err != nil {
			result <- startupResult{err: err}

			return
		}

		ack, err := sub.Receive(ctx)
		result <- startupResult{ack: ack, err: err}
	}()

	var startup startupResult
	select {
	case <-ctx.Done():
		// The cancellation callback owns cleanup; Close may wait for Subscribe.
		return nil, fmt.Errorf("subscribe to %s: %w", chKey, ctx.Err())
	case startup = <-result:
	}

	if !stop() || ctx.Err() != nil {
		closeSub()

		return nil, fmt.Errorf("subscribe to %s: %w", chKey, ctx.Err())
	}

	if startup.err != nil {
		closeSub()

		return nil, fmt.Errorf("subscribe to %s: %w", chKey, startup.err)
	}

	if confirmation, ok := startup.ack.(*redis.Subscription); !ok ||
		confirmation.Kind != "subscribe" ||
		confirmation.Channel != chKey {
		closeSub()

		return nil, fmt.Errorf("%w for %s: %v", ErrUnexpectedAcknowledgement, chKey, startup.ack)
	}

	ch := sub.Channel()

	slog.InfoContext(listenerCtx, "Subscribed to channel", slog.String("channel", chKey))
	ctx = listenerCtx //nolint:contextcheck // Listener outlives its startup context.

	go func() {
		defer closeSub()

		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					slog.WarnContext(
						ctx,
						"Redis subscription to channel is closed",
						slog.String("channel", chKey),
					)

					return
				}

				if ctx.Err() != nil {
					return
				}

				slog.InfoContext(
					ctx,
					"Received message",
					slog.String("channel", chKey),
					slog.String("message", msg.Payload),
				)

				err := handler(ctx, msg.Payload)
				if err != nil {
					slog.ErrorContext(
						ctx,
						"Error handling message",
						slog.String("channel", chKey),
						slog.String("message", msg.Payload),
						slog.Any("error", err),
					)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return closeSub, nil
}
