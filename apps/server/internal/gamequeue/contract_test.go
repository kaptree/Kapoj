package gamequeue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type handlerFunc func(context.Context, Event) error

func (f handlerFunc) HandleGameEvent(ctx context.Context, ev Event) error { return f(ctx, ev) }

func TestEventJSONContract(t *testing.T) {
	assert.Equal(t, "game:event", TypeEvent)
	assert.Equal(t, "game", QueueName)
	assert.Equal(t, EventType("submission.accepted"), EventSubmissionAccepted)
	assert.Equal(t, EventType("user.login"), EventUserLogin)
	assert.Equal(t, EventType("contest.join"), EventContestJoin)

	raw := `{"type":"submission.accepted","userId":2,"submissionId":9,"occurredAt":"2026-11-04T00:00:00Z"}`
	var ev Event
	require.NoError(t, json.Unmarshal([]byte(raw), &ev))
	assert.Equal(t, EventSubmissionAccepted, ev.Type)
	assert.EqualValues(t, 2, ev.UserID)
	assert.EqualValues(t, 9, ev.SubmissionID)
	assert.True(t, ev.OccurredAt.Equal(time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)))
}

func TestEventMarshalOmitsZeroContestID(t *testing.T) {
	payload, err := json.Marshal(Event{
		Type: EventUserLogin, UserID: 2,
		OccurredAt: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(payload, &raw))
	assert.NotContains(t, raw, "contestId")
	assert.Equal(t, "user.login", raw["type"])
	assert.EqualValues(t, 2, raw["userId"])
	assert.Equal(t, "2026-11-04T00:00:00Z", raw["occurredAt"])
}

func TestWorkerDispatchesEvent(t *testing.T) {
	var got Event
	calls := 0
	mux := asynq.NewServeMux()
	Register(mux, handlerFunc(func(_ context.Context, ev Event) error {
		calls++
		got = ev
		return nil
	}))

	payload := []byte(`{"type":"submission.accepted","userId":12,"submissionId":345,"contestId":7,"occurredAt":"2026-11-04T00:00:00Z"}`)
	require.NoError(t, mux.ProcessTask(context.Background(), asynq.NewTask(TypeEvent, payload)))

	assert.Equal(t, 1, calls)
	assert.Equal(t, EventSubmissionAccepted, got.Type)
	assert.EqualValues(t, 12, got.UserID)
	assert.EqualValues(t, 345, got.SubmissionID)
	assert.EqualValues(t, 7, got.ContestID)
}

func TestWorkerRejectsMalformedPayload(t *testing.T) {
	mux := asynq.NewServeMux()
	Register(mux, handlerFunc(func(context.Context, Event) error { return nil }))

	err := mux.ProcessTask(context.Background(), asynq.NewTask(TypeEvent, []byte(`{"type":`)))
	assert.Error(t, err)
}

func TestWorkerRejectsMissingUserID(t *testing.T) {
	mux := asynq.NewServeMux()
	Register(mux, handlerFunc(func(context.Context, Event) error { return nil }))

	err := mux.ProcessTask(context.Background(), asynq.NewTask(TypeEvent, []byte(`{"type":"user.login"}`)))
	assert.Error(t, err)
}

func TestTaskContract(t *testing.T) {
	task, err := newTask(Event{Type: EventSubmissionAccepted, UserID: 12, SubmissionID: 345})
	require.NoError(t, err)
	assert.Equal(t, TypeEvent, task.Type())

	var ev Event
	require.NoError(t, json.Unmarshal(task.Payload(), &ev))
	assert.Equal(t, EventSubmissionAccepted, ev.Type)
	assert.EqualValues(t, 12, ev.UserID)
	assert.EqualValues(t, 345, ev.SubmissionID)
	assert.False(t, ev.OccurredAt.IsZero(), "occurredAt must be filled when omitted")
}
