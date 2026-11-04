// Package submissionevents bridges judge events from Redis to SSE clients.
package submissionevents

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

const Channel = "kapoj:submissions"

type Event struct {
	SubmissionID int64 `json:"submissionId"`
}

type Streamer interface {
	Subscribe(ctx context.Context) (<-chan Event, error)
}

type RedisStreamer struct{ rdb *redis.Client }

func NewRedisStreamer(rdb *redis.Client) *RedisStreamer {
	return &RedisStreamer{rdb: rdb}
}

func (s *RedisStreamer) Subscribe(ctx context.Context) (<-chan Event, error) {
	sub := s.rdb.Subscribe(ctx, Channel)
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, err
	}
	out := make(chan Event)
	go func() {
		defer close(out)
		defer func() { _ = sub.Close() }()
		messages := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-messages:
				if !ok {
					return
				}
				var event Event
				if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil || event.SubmissionID <= 0 {
					continue
				}
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
