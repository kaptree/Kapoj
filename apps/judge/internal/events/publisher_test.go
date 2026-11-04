package events_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/events"
)

func TestRedisPublisherContract(t *testing.T) {
	assert.Equal(t, "kapoj:submissions", events.Channel)

	mr := miniredis.RunT(t)
	subscriber := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = subscriber.Close() })
	pubsub := subscriber.Subscribe(context.Background(), events.Channel)
	t.Cleanup(func() { _ = pubsub.Close() })
	_, err := pubsub.Receive(context.Background())
	require.NoError(t, err)

	publisher := events.NewRedisPublisher(mr.Addr())
	t.Cleanup(func() { _ = publisher.Close() })
	publisher.Publish(context.Background(), 42)

	select {
	case msg := <-pubsub.Channel():
		var event events.Event
		require.NoError(t, json.Unmarshal([]byte(msg.Payload), &event))
		assert.Equal(t, int64(42), event.SubmissionID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for published event")
	}
}
