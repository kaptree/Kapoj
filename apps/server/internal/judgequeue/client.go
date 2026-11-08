// Package judgequeue owns the Asynq contract shared with the judge process.
package judgequeue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TypeSubmission         = "judge:submission"
	TypeTestcaseGeneration = "judge:testcase-generation"
	QueueName              = "judge"
)

// Payload is the JSON body of a judge task. Keep it in sync with apps/judge.
type Payload struct {
	SubmissionID int64 `json:"submissionId"`
}

// TestcaseGenerationPayload is the JSON body of an AI testcase generation task.
type TestcaseGenerationPayload struct {
	GenerationID int64 `json:"generationId"`
}

type Client struct{ client *asynq.Client }

func NewClient(redisAddr string) *Client {
	return &Client{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

func (c *Client) Close() error { return c.client.Close() }

func (c *Client) Enqueue(ctx context.Context, submissionID int64) error {
	_, err := c.client.EnqueueContext(ctx, newTask(submissionID))
	return err
}

// EnqueueTestcaseGeneration queues an AI testcase generation for the judge.
func (c *Client) EnqueueTestcaseGeneration(ctx context.Context, generationID int64) error {
	_, err := c.client.EnqueueContext(ctx, newGenerationTask(generationID))
	return err
}

func newTask(submissionID int64) *asynq.Task {
	payload, _ := json.Marshal(Payload{SubmissionID: submissionID})
	return asynq.NewTask(TypeSubmission, payload,
		asynq.Queue(QueueName), asynq.MaxRetry(3), asynq.Timeout(10*time.Minute))
}

func newGenerationTask(generationID int64) *asynq.Task {
	payload, _ := json.Marshal(TestcaseGenerationPayload{GenerationID: generationID})
	return asynq.NewTask(TypeTestcaseGeneration, payload,
		asynq.Queue(QueueName), asynq.MaxRetry(0), asynq.Timeout(30*time.Minute))
}
