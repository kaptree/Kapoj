package gamequeue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// Handler consumes gamification events. Returning an error asks Asynq to
// retry; gamification.ErrSubmissionNotFound is handled as a no-op by the
// gamification service so stale events are not retried.
type Handler interface {
	HandleGameEvent(ctx context.Context, ev Event) error
}

func Register(mux *asynq.ServeMux, h Handler) {
	mux.HandleFunc(TypeEvent, func(ctx context.Context, task *asynq.Task) error {
		var ev Event
		if err := json.Unmarshal(task.Payload(), &ev); err != nil {
			return fmt.Errorf("decode game event: %w", err)
		}
		if ev.UserID <= 0 {
			return fmt.Errorf("game event %q has invalid user id %d", ev.Type, ev.UserID)
		}
		return h.HandleGameEvent(ctx, ev)
	})
}
