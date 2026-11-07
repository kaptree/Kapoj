package learning

import (
	"context"

	"github.com/kaptree/kapoj/apps/server/internal/gamequeue"
)

// HandleGameEvent recomputes a user's learning progress when a submission is
// accepted. It only reacts to submission.accepted; other event types and
// non-positive user ids are no-ops.
func (s *Service) HandleGameEvent(ctx context.Context, ev gamequeue.Event) error {
	switch ev.Type {
	case gamequeue.EventSubmissionAccepted:
		if ev.UserID <= 0 {
			return nil
		}
		return s.EvaluateUser(ctx, ev.UserID)
	}
	return nil
}
