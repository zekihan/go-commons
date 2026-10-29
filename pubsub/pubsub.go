package pubsub

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

var (
	ErrDeadlineRequired          = errors.New("listen requires a context deadline")
	ErrReadinessUnsupported      = errors.New("pubsub backend does not support readiness")
	ErrUnexpectedAcknowledgement = errors.New("unexpected subscription acknowledgement")
)

type PubSuber interface {
	Publish(ctx context.Context, channel, message string) error
	Listen(
		channel string,
		handler func(ctx context.Context, payload string) error,
	) context.CancelFunc
}

type UniversalPubSub struct {
	PubSuber
}

type UniversalPubSubOptions struct {
	Redis  redis.UniversalClient
	Prefix string
}

func NewUniversalPubSub(opts *UniversalPubSubOptions) *UniversalPubSub {
	if opts == nil {
		opts = &UniversalPubSubOptions{}
	}

	var ps PubSuber
	if opts.Redis != nil {
		ps = NewRedisPubSub(opts)
	} else {
		ps = NewLocalPubSub(opts)
	}

	return &UniversalPubSub{
		PubSuber: ps,
	}
}

func getChannelName(prefix, channel string) string {
	if prefix != "" {
		return prefix + ":" + channel
	}

	return channel
}

// ListenReady waits for listener initialization. Ctx must have a deadline and only
// applies to initialization. Cancel the returned function to stop the listener.
func (ps *UniversalPubSub) ListenReady(ctx context.Context, channel string,
	handler func(context.Context, string) error,
) (context.CancelFunc, error) {
	ready, ok := ps.PubSuber.(interface {
		ListenReady(ctx context.Context, channel string, handler func(context.Context, string) error) (context.CancelFunc, error) //nolint:lll // Interface signature remains readable.
	})
	if !ok {
		return nil, ErrReadinessUnsupported
	}

	cancel, err := ready.ListenReady(ctx, channel, handler)
	if err != nil {
		return nil, fmt.Errorf("initialize listener: %w", err)
	}

	return cancel, nil
}
