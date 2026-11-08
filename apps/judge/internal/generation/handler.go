package generation

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/kaptree/kapoj/apps/judge/internal/queue"
)

// Handle consumes a judge:testcase-generation task. Malformed payloads are
// ignored (the generation simply stays pending for the admin to re-trigger).
func (p *Pipeline) Handle(ctx context.Context, task *asynq.Task) error {
	var payload queue.TestcaseGenerationPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil || payload.GenerationID <= 0 {
		slog.Warn("ignoring malformed testcase-generation task", "type", task.Type())
		return nil
	}
	if err := p.Run(ctx, payload.GenerationID); err != nil {
		slog.Warn("testcase generation failed", "generation_id", payload.GenerationID, "err", err)
		return err
	}
	return nil
}
