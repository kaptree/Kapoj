package agent

import (
	"time"

	"gorm.io/gorm"
)

// ApplySolveForTest exposes the settlement helper to package agent tests.
func (s *Service) ApplySolveForTest(
	tx *gorm.DB, a *Agent, attempt *AgentProblemAttempt, submissionID int64, now time.Time,
) error {
	return s.applySolve(tx, a, attempt, submissionID, now)
}

// ApplyFailureForTest exposes the failure helper to package agent tests.
func (s *Service) ApplyFailureForTest(
	tx *gorm.DB, a *Agent, attempt *AgentProblemAttempt, submissionID *int64, kind RatingKind, now time.Time,
) (*int, error) {
	return s.applyFailure(tx, a, attempt, submissionID, kind, now)
}
