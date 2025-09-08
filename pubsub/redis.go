package pubsub

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	log "github.com/sirupsen/logrus"
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

	if ps.rdb == nil {
		log.Warnf("Redis is not enabled, cannot publish message to %s", chKey)

		return nil
	}

	err := ps.rdb.Publish(ctx, chKey, message).Err()
	if err != nil {
		return fmt.Errorf("error publishing message to %s: %w", chKey, err)
	}

	log.Infof("Published message to %s: %s", chKey, message)

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

	log.Infof("Subscribed to %s", chKey)

	go func() {
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					log.Warnf("Redis subscription to %s channel closed", chKey)

					return
				}

				log.Infof("Received message on %s: %v\n", chKey, msg.Payload)

				err := handler(ctx, msg.Payload)
				if err != nil {
					log.Errorf("Error executing function for %s: %v", chKey, err)
				}
			case <-ctx.Done():
				log.Infof("Unsubscribing from %s", chKey)

				err := sub.Close()
				if err != nil {
					log.Errorf("Error unsubscribing from %s: %v", chKey, err)

					return
				}

				log.Infof("Unsubscribed from %s", chKey)

				return
			}
		}
	}()

	return cancel
}
