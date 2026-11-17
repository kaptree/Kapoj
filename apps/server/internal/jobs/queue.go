package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

const (
	// QueueName is the dedicated Asynq queue for scheduled jobs.
	QueueName = "jobs"
	// TypeRun is the task type that executes one persisted job run.
	TypeRun = "jobs.run"
)

type taskPayload struct {
	RunID int64 `json:"runId"`
}

// Queue is the Asynq producer for scheduled job runs.
type Queue struct {
	client *asynq.Client
}

var _ Enqueuer = (*Queue)(nil)

// NewQueue returns a Queue connected to redisAddr.
func NewQueue(redisAddr string) *Queue {
	return &Queue{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

// Close releases the underlying Redis client.
func (q *Queue) Close() error { return q.client.Close() }

// Enqueue schedules one run for asynchronous execution. Runs are never retried
// automatically: a failed run keeps its recorded terminal state.
func (q *Queue) Enqueue(ctx context.Context, runID int64) error {
	payload, _ := json.Marshal(taskPayload{RunID: runID})
	_, err := q.client.EnqueueContext(ctx, asynq.NewTask(TypeRun, payload),
		asynq.Queue(QueueName), asynq.MaxRetry(0), asynq.Timeout(6*time.Hour))
	return err
}

// Register installs the scheduled job task handler.
func Register(mux *asynq.ServeMux, svc *Service) {
	mux.HandleFunc(TypeRun, func(ctx context.Context, task *asynq.Task) error {
		var payload taskPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		return svc.Execute(ctx, payload.RunID)
	})
}
