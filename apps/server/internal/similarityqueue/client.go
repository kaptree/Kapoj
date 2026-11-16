package similarityqueue

import (
	"context"

	"github.com/hibiken/asynq"
)

// Client is the Asynq producer for similarity tasks.
type Client struct{ client *asynq.Client }

// NewClient returns a Client connected to the given Redis address.
func NewClient(redisAddr string) *Client {
	return &Client{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

// Close releases the underlying Redis connection.
func (c *Client) Close() error { return c.client.Close() }

// Enqueue schedules one similarity run.
func (c *Client) Enqueue(ctx context.Context, runID int64) error {
	_, err := c.client.EnqueueContext(ctx, newTask(runID))
	return err
}
