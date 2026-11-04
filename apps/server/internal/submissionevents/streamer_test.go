package submissionevents_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/submissionevents"
)

func TestRedisStreamerDeliversPublishedEvents(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	streamer := submissionevents.NewRedisStreamer(rdb)
	events, err := streamer.Subscribe(ctx)
	require.NoError(t, err)

	require.NoError(t, rdb.Publish(ctx, submissionevents.Channel, `{"submissionId":42}`).Err())
	select {
	case event := <-events:
		assert.Equal(t, int64(42), event.SubmissionID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}

	cancel()
	_, ok := <-events
	assert.False(t, ok, "canceling the context must close the stream")
}
