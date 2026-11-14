package backup

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

const (
	QueueName   = "backup"
	TypeCreate  = "backup.create"
	TypeRestore = "backup.restore"
)

type taskPayload struct {
	ID int64 `json:"id"`
}

// Client is the Asynq producer for backup tasks.
type Client struct{ client *asynq.Client }

func NewClient(redisAddr string) *Client {
	return &Client{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

func (c *Client) Close() error { return c.client.Close() }

func (c *Client) EnqueueCreate(ctx context.Context, packageID int64) error {
	payload, _ := json.Marshal(taskPayload{ID: packageID})
	_, err := c.client.EnqueueContext(ctx, asynq.NewTask(TypeCreate, payload),
		asynq.Queue(QueueName), asynq.MaxRetry(0), asynq.Timeout(6*time.Hour))
	return err
}

func (c *Client) EnqueueRestore(ctx context.Context, jobID int64) error {
	payload, _ := json.Marshal(taskPayload{ID: jobID})
	_, err := c.client.EnqueueContext(ctx, asynq.NewTask(TypeRestore, payload),
		asynq.Queue(QueueName), asynq.MaxRetry(0), asynq.Timeout(6*time.Hour))
	return err
}

// Register installs backup task handlers.
func Register(mux *asynq.ServeMux, svc *Service) {
	mux.HandleFunc(TypeCreate, func(ctx context.Context, task *asynq.Task) error {
		var payload taskPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		return svc.RunPackage(ctx, payload.ID)
	})
	mux.HandleFunc(TypeRestore, func(ctx context.Context, task *asynq.Task) error {
		var payload taskPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		if err := svc.RunRestore(ctx, payload.ID); err != nil {
			slog.Warn("backup: restore failed", "job", payload.ID, "err", err)
			return err
		}
		return nil
	})
}
