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
