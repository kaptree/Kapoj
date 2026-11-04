package gamequeue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

type Publisher struct{ client *asynq.Client }

func NewPublisher(redisAddr string) *Publisher {
	return &Publisher{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

func (p *Publisher) Close() error { return p.client.Close() }

func (p *Publisher) Publish(ctx context.Context, ev Event) error {
	task, err := newTask(ev)
	if err != nil {
		return err
	}
	_, err = p.client.EnqueueContext(ctx, task)
	return err
}

func newTask(ev Event) (*asynq.Task, error) {
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeEvent, payload,
		asynq.Queue(QueueName), asynq.MaxRetry(maxRetries), asynq.Timeout(eventTimeout)), nil
}
