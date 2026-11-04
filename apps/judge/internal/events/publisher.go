// Package events publishes submission state changes to the Web service.
package events

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

const Channel = "kapoj:submissions"

type Event struct {
	SubmissionID int64 `json:"submissionId"`
}

type Publisher interface {
	Publish(ctx context.Context, submissionID int64)
}

type RedisPublisher struct{ rdb *redis.Client }

func NewRedisPublisher(addr string) *RedisPublisher {
	return &RedisPublisher{rdb: redis.NewClient(&redis.Options{Addr: addr})}
}

func (p *RedisPublisher) Close() error { return p.rdb.Close() }

func (p *RedisPublisher) Publish(ctx context.Context, submissionID int64) {
	data, err := json.Marshal(Event{SubmissionID: submissionID})
	if err != nil {
		return
	}
	if err := p.rdb.Publish(ctx, Channel, data).Err(); err != nil {
		slog.Warn("publish submission event failed", "submission_id", submissionID, "err", err)
	}
}
