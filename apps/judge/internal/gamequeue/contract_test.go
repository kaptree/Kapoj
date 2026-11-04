package gamequeue

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestEventMarshalOmitsZeroContestID(t *testing.T) {
	payload, err := json.Marshal(Event{
		Type: EventSubmissionAccepted, UserID: 2, SubmissionID: 9,
		OccurredAt: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(payload, &raw))
	assert.NotContains(t, raw, "contestId")
	assert.EqualValues(t, 9, raw["submissionId"])
}
