package pubsub

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

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
	chKey := getChannelName(ps.prefix, channel)

	ctx, cancel := context.WithCancel(context.Background())
	sub := ps.rdb.Subscribe(ctx, chKey)
	ch := sub.Channel()

	slog.InfoContext(ctx, "Subscribed to channel", slog.String("channel", chKey))

	go func() {
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
				slog.InfoContext(
					ctx, "Unsubscribing from channel", slog.String("channel", chKey),
				)

				err := sub.Close()
				if err != nil {
					slog.ErrorContext(
						ctx,
						"Error unsubscribing from channel",
						slog.String("channel", chKey),
						slog.Any("error", err),
					)

					return
				}

				slog.InfoContext(
					ctx, "Unsubscribed from channel", slog.String("channel", chKey),
				)

				return
			}
		}
	}()

	return cancel
}
