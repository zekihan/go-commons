package pubsub

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
)

type LocalPubSub struct {
	ch     map[string]chan string
	prefix string
	mu     sync.RWMutex
}

func NewLocalPubSub(opts *UniversalPubSubOptions) *LocalPubSub {
	if opts == nil {
		opts = &UniversalPubSubOptions{}
	}

	return &LocalPubSub{
		ch:     make(map[string]chan string),
		prefix: opts.Prefix,
	}
}

func (ps *LocalPubSub) Publish(_ context.Context, channel, message string) error {
	chKey := getChannelName(ps.prefix, channel)

	ch := ps.getChannel(chKey)

	if ch == nil {
		log.Warnf("channel %s not found", chKey)

		return nil
	}

	ch <- message

	return nil
}

func (ps *LocalPubSub) Listen(
	channel string,
	handler func(ctx context.Context, payload string) error,
) context.CancelFunc {
	chKey := getChannelName(ps.prefix, channel)

	ctx, cancel := context.WithCancel(context.Background())
	ch := ps.getOrCreateChannel(chKey)
	log.Infof("Subscribed to %s", chKey)

	go func() {
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					log.Warnf("Redis subscription to %s channel closed", chKey)

					return
				}

				log.Infof("Received message on %s: %v\n", chKey, msg)

				err := handler(ctx, msg)
				if err != nil {
					log.Errorf("Error executing function for %s: %v", chKey, err)
				}
			case <-ctx.Done():
				log.Infof("Unsubscribing from %s", chKey)
				ps.mu.Lock()
				close(ch)
				delete(ps.ch, chKey)
				ps.mu.Unlock()
				log.Infof("Unsubscribed from %s", chKey)

				return
			}
		}
	}()

	return cancel
}

func (ps *LocalPubSub) getOrCreateChannel(channel string) chan string {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.ch == nil {
		ps.ch = make(map[string]chan string)
	}

	if _, ok := ps.ch[channel]; !ok {
		ps.ch[channel] = make(chan string)
	}

	return ps.ch[channel]
}

func (ps *LocalPubSub) getChannel(channel string) chan string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.ch == nil {
		return nil
	}

	return ps.ch[channel]
}
