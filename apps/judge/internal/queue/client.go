package queue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

type Client struct{ client *asynq.Client }

func NewClient(redisAddr string) *Client {
	return &Client{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

func (c *Client) Close() error { return c.client.Close() }

func (c *Client) Enqueue(ctx context.Context, submissionID int64) error {
	_, err := c.client.EnqueueContext(ctx, newTask(submissionID))
	return err
}

func newTask(submissionID int64) *asynq.Task {
	payload, _ := json.Marshal(Payload{SubmissionID: submissionID})
	return asynq.NewTask(TypeSubmission, payload,
		asynq.Queue(QueueName), asynq.MaxRetry(3), asynq.Timeout(10*time.Minute))
}
