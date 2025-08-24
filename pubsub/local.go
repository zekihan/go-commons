package pubsub

import (
	"context"

	log "github.com/sirupsen/logrus"
)

type LocalPubSub struct {
	ch map[string]chan string
}

func NewLocalPubSub() *LocalPubSub {
	return &LocalPubSub{
		ch: make(map[string]chan string),
	}
}

func (ps *LocalPubSub) Publish(_ context.Context, channel, message string) error {
	ch := ps.getChannel(channel)

	if ch == nil {
		log.Warnf("channel %s not found", channel)

		return nil
	}

	ch <- message

	return nil
}

func (ps *LocalPubSub) Listen(
	channel string,
	handler func(ctx context.Context, payload string) error,
) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	ch := ps.getOrCreateChannel(channel)
	log.Infof("Subscribed to %s", channel)

	go func() {
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					log.Warnf("Redis subscription to %s channel closed", channel)

					return
				}

				log.Infof("Received message on %s: %v\n", channel, msg)

				err := handler(ctx, msg)
				if err != nil {
					log.Errorf("Error executing function for %s: %v", channel, err)
				}
			case <-ctx.Done():
				log.Infof("Unsubscribing from %s", channel)
				close(ch)
				delete(ps.ch, channel)
				log.Infof("Unsubscribed from %s", channel)

				return
			}
		}
	}()

	return cancel
}

func (ps *LocalPubSub) getOrCreateChannel(channel string) chan string {
	if ps.ch == nil {
		ps.ch = make(map[string]chan string)
	}

	if _, ok := ps.ch[channel]; !ok {
		ps.ch[channel] = make(chan string)
	}

	return ps.ch[channel]
}

func (ps *LocalPubSub) getChannel(channel string) chan string {
	if ps.ch == nil {
		ps.ch = make(map[string]chan string)
	}

	return ps.ch[channel]
}
