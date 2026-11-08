package queue

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContractMatchesWebService(t *testing.T) {
	assert.Equal(t, "judge:submission", TypeSubmission)
	assert.Equal(t, "judge", QueueName)

	var payload Payload
	require.NoError(t, json.Unmarshal([]byte(`{"submissionId":42}`), &payload))
	assert.Equal(t, int64(42), payload.SubmissionID)
}

func TestTestcaseGenerationContractMatchesWebService(t *testing.T) {
	assert.Equal(t, "judge:testcase-generation", TypeTestcaseGeneration)

	var payload TestcaseGenerationPayload
	require.NoError(t, json.Unmarshal([]byte(`{"generationId":7}`), &payload))
	assert.Equal(t, int64(7), payload.GenerationID)

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"generationId":7}`), &raw))
	assert.Equal(t, map[string]any{"generationId": float64(7)}, raw, "payload must carry only generationId")
}
