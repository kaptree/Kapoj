// Package similarityqueue owns the Asynq contract for asynchronous
// similarity runs.
package similarityqueue

import (
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

const (
	// QueueName is the dedicated queue so similarity runs never compete with
	// judging for worker capacity.
	QueueName = "similarity"
	// TypeRun processes one similarity run.
	TypeRun = "similarity:run"
)

// Payload is the JSON body of a similarity run task.
type Payload struct {
	RunID int64 `json:"runId"`
}

func newTask(runID int64) *asynq.Task {
	payload, _ := json.Marshal(Payload{RunID: runID})
	return asynq.NewTask(TypeRun, payload,
		asynq.Queue(QueueName), asynq.MaxRetry(1), asynq.Timeout(10*time.Minute))
}
