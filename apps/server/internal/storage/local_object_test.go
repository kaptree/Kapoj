package storage

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalListAndDeletePrefix(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	require.NoError(t, err)
	ctx := context.Background()
	for _, key := range []string{"a/b", "a/c", "d"} {
		_, err := store.Put(ctx, key, strings.NewReader(key), 100)
		require.NoError(t, err)
	}

	objects, err := store.List(ctx, "a/")
	require.NoError(t, err)
	assert.Equal(t, []ObjectInfo{
		{Key: "a/b", Size: 3},
		{Key: "a/c", Size: 3},
	}, objects)

	require.NoError(t, store.DeletePrefix(ctx, "a"))
	objects, err = store.List(ctx, "a/")
	require.NoError(t, err)
	assert.Empty(t, objects)

	require.NoError(t, store.DeletePrefix(ctx, ""))
	objects, err = store.List(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, objects)
}

func TestValidateKey(t *testing.T) {
	for _, key := range []string{"badges/1", "avatars/33/hash", "problems/1/cases/0.in"} {
		require.NoError(t, ValidateKey(key), key)
	}
	for _, key := range []string{"", "/abs", "../x", "a/../b", `a\b`, "a//b", "."} {
		require.ErrorIs(t, ValidateKey(key), ErrInvalidKey, key)
	}
}
