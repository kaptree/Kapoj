package generation_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/model"
	"github.com/kaptree/kapoj/apps/judge/internal/queue"
)

func TestHandleIgnoresMalformedPayload(t *testing.T) {
	e := newGenEnv(t)

	require.NoError(t, e.pipe.Handle(context.Background(),
		asynq.NewTask(queue.TypeTestcaseGeneration, []byte("not-json"))))
	require.NoError(t, e.pipe.Handle(context.Background(),
		asynq.NewTask(queue.TypeTestcaseGeneration, []byte(`{"generationId":0}`))))

	assert.Empty(t, e.sb.compiles)
	assert.Empty(t, e.sb.runs)
}

func TestHandleRunsPipeline(t *testing.T) {
	e := newGenEnv(t)
	id := e.insert(t, "cpp", "int main(){}", "print(1)", 1, "pending")

	task := asynq.NewTask(queue.TypeTestcaseGeneration, []byte(`{"generationId":`+strconv.FormatInt(id, 10)+`}`))
	require.NoError(t, e.pipe.Handle(context.Background(), task))

	status, _ := genStatus(t, e.pool, id)
	assert.Equal(t, model.GenerationReady, status)
}
