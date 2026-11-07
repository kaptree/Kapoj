package gamequeue

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChainRunsInOrderAndStopsOnError(t *testing.T) {
	t.Run("runs in order", func(t *testing.T) {
		var order []int
		chain := NewChain(
			handlerFunc(func(context.Context, Event) error { order = append(order, 1); return nil }),
			handlerFunc(func(context.Context, Event) error { order = append(order, 2); return nil }),
			handlerFunc(func(context.Context, Event) error { order = append(order, 3); return nil }),
		)
		require.NoError(t, chain.HandleGameEvent(context.Background(), Event{Type: EventUserLogin, UserID: 1}))
		assert.Equal(t, []int{1, 2, 3}, order)
	})

	t.Run("stops on first error", func(t *testing.T) {
		boom := errors.New("boom")
		var order []int
		chain := NewChain(
			handlerFunc(func(context.Context, Event) error { order = append(order, 1); return nil }),
			handlerFunc(func(context.Context, Event) error { order = append(order, 2); return boom }),
			handlerFunc(func(context.Context, Event) error { order = append(order, 3); return nil }),
		)
		err := chain.HandleGameEvent(context.Background(), Event{Type: EventUserLogin, UserID: 1})
		require.ErrorIs(t, err, boom)
		assert.Equal(t, []int{1, 2}, order, "第一个返回错误的处理器之后不再调用")
	})
}

func TestChainSkipsNilHandlers(t *testing.T) {
	var order []int
	chain := NewChain(
		nil,
		handlerFunc(func(context.Context, Event) error { order = append(order, 1); return nil }),
		nil,
		handlerFunc(func(context.Context, Event) error { order = append(order, 2); return nil }),
	)
	require.NoError(t, chain.HandleGameEvent(context.Background(), Event{Type: EventUserLogin, UserID: 1}))
	assert.Equal(t, []int{1, 2}, order)
}
