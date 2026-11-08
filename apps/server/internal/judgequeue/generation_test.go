package judgequeue

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// taskOption reads an unexported asynq.Task option of the given type. Test-only:
// asynq does not expose task options, so we reach into the struct by reflection.
func taskOption(task *asynq.Task, typ asynq.OptionType) (any, bool) {
	rv := reflect.ValueOf(task).Elem()
	field := rv.FieldByName("opts")
	opts := *(*[]asynq.Option)(unsafe.Pointer(field.UnsafeAddr()))
	for _, o := range opts {
		if o.Type() == typ {
			return o.Value(), true
		}
	}
	return nil, false
}

func TestGenerationTaskPayloadContract(t *testing.T) {
	assert.Equal(t, "judge:testcase-generation", TypeTestcaseGeneration)

	task := newGenerationTask(7)
	assert.Equal(t, TypeTestcaseGeneration, task.Type())

	var payload TestcaseGenerationPayload
	require.NoError(t, json.Unmarshal(task.Payload(), &payload))
	assert.Equal(t, int64(7), payload.GenerationID)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(task.Payload(), &raw))
	assert.Equal(t, map[string]any{"generationId": float64(7)}, raw, "payload must carry only generationId")

	queue, ok := taskOption(task, asynq.QueueOpt)
	require.True(t, ok, "task must specify a queue")
	assert.Equal(t, QueueName, queue)

	retry, ok := taskOption(task, asynq.MaxRetryOpt)
	require.True(t, ok, "task must specify max retry")
	assert.Equal(t, 0, retry, "generation tasks must not be retried")

	timeout, ok := taskOption(task, asynq.TimeoutOpt)
	require.True(t, ok, "task must specify a timeout")
	assert.Equal(t, 30*time.Minute, timeout)
}
