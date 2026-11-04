// Package worker consumes judge tasks from Asynq.
package worker

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/kaptree/kapoj/apps/judge/internal/pipeline"
	"github.com/kaptree/kapoj/apps/judge/internal/queue"
)

type Worker struct{ pipeline *pipeline.Pipeline }

func New(p *pipeline.Pipeline) *Worker { return &Worker{pipeline: p} }

func (w *Worker) Handle(ctx context.Context, task *asynq.Task) error {
	var payload queue.Payload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil || payload.SubmissionID <= 0 {
		slog.Warn("ignoring malformed judge task", "type", task.Type())
		return nil
	}
	retry, _ := asynq.GetRetryCount(ctx)
	maxRetry, ok := asynq.GetMaxRetry(ctx)
	if !ok {
		maxRetry = 3
	}
	return w.handle(ctx, payload.SubmissionID, retry, maxRetry)
}

func (w *Worker) handle(ctx context.Context, submissionID int64, retry, maxRetry int) error {
	err := w.pipeline.Run(ctx, submissionID)
	if err == nil {
		return nil
	}
	if retry >= maxRetry {
		w.pipeline.MarkSystemError(ctx, submissionID, err)
		return nil
	}
	slog.Warn("judge attempt failed", "submission_id", submissionID, "retry", retry, "err", err)
	return err
}
