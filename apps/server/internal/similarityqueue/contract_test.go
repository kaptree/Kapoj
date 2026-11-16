package similarityqueue

import (
	"context"
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
func taskOption(task *asynq.Task, typ asynq.OptionType) any {
	rv := reflect.ValueOf(task).Elem()
	field := rv.FieldByName("opts")
	opts := *(*[]asynq.Option)(unsafe.Pointer(field.UnsafeAddr()))
	for _, o := range opts {
		if o.Type() == typ {
			return o.Value()
		}
	}
	return nil
}

type fakeHandler struct{ runID int64 }

func (f *fakeHandler) ProcessRun(_ context.Context, runID int64) error {
	f.runID = runID
	return nil
}

func TestContract(t *testing.T) {
	task := newTask(42)
	assert.Equal(t, TypeRun, task.Type())
	assert.Equal(t, []byte(`{"runId":42}`), task.Payload())
	assert.Equal(t, QueueName, taskOption(task, asynq.QueueOpt))
	assert.Equal(t, 10*time.Minute, taskOption(task, asynq.TimeoutOpt))
	assert.Equal(t, 1, taskOption(task, asynq.MaxRetryOpt))
}

func TestWorkerCallsService(t *testing.T) {
	fake := &fakeHandler{}
	mux := asynq.NewServeMux()
	Register(mux, fake)
	require.NoError(t, mux.ProcessTask(context.Background(), newTask(7)))
	assert.Equal(t, int64(7), fake.runID)
}
