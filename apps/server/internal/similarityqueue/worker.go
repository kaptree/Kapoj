package similarityqueue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// Handler consumes similarity run tasks.
type Handler interface {
	ProcessRun(ctx context.Context, runID int64) error
}

// Register installs the similarity run handler on mux.
func Register(mux *asynq.ServeMux, h Handler) {
	mux.HandleFunc(TypeRun, func(ctx context.Context, task *asynq.Task) error {
		var payload Payload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("decode similarity run: %w", err)
		}
		if payload.RunID <= 0 {
			return fmt.Errorf("similarity run has invalid id %d", payload.RunID)
		}
		return h.ProcessRun(ctx, payload.RunID)
	})
}
