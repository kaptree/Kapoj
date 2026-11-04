// Package queue holds the Asynq contract shared with the Web service.
package queue

const (
	TypeSubmission = "judge:submission"
	QueueName      = "judge"
)

// Payload is the JSON body of a judge task. Keep it in sync with apps/server.
type Payload struct {
	SubmissionID int64 `json:"submissionId"`
}
