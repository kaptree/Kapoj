package submissionevents

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

type Publisher interface {
	Publish(ctx context.Context, submissionID int64)
}

type RedisPublisher struct{ rdb *redis.Client }

func NewRedisPublisher(rdb *redis.Client) *RedisPublisher {
	return &RedisPublisher{rdb: rdb}
}

func (p *RedisPublisher) Publish(ctx context.Context, submissionID int64) {
	data, err := json.Marshal(Event{SubmissionID: submissionID})
	if err != nil {
		return
	}
	_ = p.rdb.Publish(ctx, Channel, data).Err()
}
