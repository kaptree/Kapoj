// Package gamequeue publishes gamification events from the judge. Keep the
// contract in sync with apps/server.
package gamequeue

import "time"

const (
	TypeEvent = "game:event"
	QueueName = "game"

	maxRetries   = 5
	eventTimeout = 30 * time.Second
)

type EventType string

const (
	EventSubmissionAccepted EventType = "submission.accepted"
	EventUserLogin          EventType = "user.login"
	EventContestJoin        EventType = "contest.join"
)

// Event is the JSON body of a game task. Events are hints, not the source of
// truth: the server worker re-reads the database and recomputes rewards.
type Event struct {
	Type         EventType `json:"type"`
	UserID       int64     `json:"userId"`
	SubmissionID int64     `json:"submissionId"`
	ContestID    int64     `json:"contestId,omitempty"`
	OccurredAt   time.Time `json:"occurredAt"`
}
