package pubsub

import (
	"context"

	"github.com/redis/go-redis/v9"
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
