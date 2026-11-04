package judgequeue

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPayloadContract(t *testing.T) {
	assert.Equal(t, "judge:submission", TypeSubmission)
	assert.Equal(t, "judge", QueueName)

	task := newTask(42)
	assert.Equal(t, TypeSubmission, task.Type())

	var payload Payload
	require.NoError(t, json.Unmarshal(task.Payload(), &payload))
	assert.Equal(t, int64(42), payload.SubmissionID)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(task.Payload(), &raw))
	assert.Equal(t, map[string]any{"submissionId": float64(42)}, raw)
}
