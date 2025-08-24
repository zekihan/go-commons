package pubsub

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	log "github.com/sirupsen/logrus"
)

type RedisPubSub struct {
	rdb redis.UniversalClient
}

func NewRedisPubSub(rdb redis.UniversalClient) *RedisPubSub {
	return &RedisPubSub{
		rdb: rdb,
	}
}

func (ps *RedisPubSub) Publish(ctx context.Context, channel, message string) error {
	if ps.rdb == nil {
		log.Warnf("Redis is not enabled, cannot publish message to %s", channel)

		return nil
	}

	err := ps.rdb.Publish(ctx, channel, message).Err()
	if err != nil {
		return fmt.Errorf("error publishing message to %s: %w", channel, err)
	}

	log.Infof("Published message to %s: %s", channel, message)

	return nil
}

func (ps *RedisPubSub) Listen(
	channel string,
	handler func(ctx context.Context, payload string) error,
) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	sub := ps.rdb.Subscribe(ctx, channel)
	ch := sub.Channel()

	log.Infof("Subscribed to %s", channel)

	go func() {
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					log.Warnf("Redis subscription to %s channel closed", channel)

					return
				}

				log.Infof("Received message on %s: %v\n", channel, msg.Payload)

				err := handler(ctx, msg.Payload)
				if err != nil {
					log.Errorf("Error executing function for %s: %v", channel, err)
				}
			case <-ctx.Done():
				log.Infof("Unsubscribing from %s", channel)
				log.Infof("Unsubscribed from %s", channel)

				return
			}
		}
	}()

	return cancel
}
