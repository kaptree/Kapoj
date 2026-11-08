package storage_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/storage"
)

func TestPutOpenRoundTrip(t *testing.T) {
	root := t.TempDir()
	n, err := storage.Put(root, "problems/7/generated/3/1.in", bytes.NewReader([]byte("1 2\n")), 16)
	require.NoError(t, err)
	assert.Equal(t, int64(4), n)

	rc, err := storage.Open(root, "problems/7/generated/3/1.in")
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "1 2\n", string(got))
}

func TestPutRejectsUnsafeKeys(t *testing.T) {
	root := t.TempDir()
	keys := []string{"", "/etc/passwd", "../secret", "a/../../b", `a\b`, "a//b", "./a", "."}
	for _, key := range keys {
		_, err := storage.Put(root, key, strings.NewReader("x"), 1)
		assert.ErrorIs(t, err, storage.ErrInvalidKey, key)
	}
}

func TestPutTooLargeLeavesExisting(t *testing.T) {
	root := t.TempDir()
	key := "problems/1/generated/1/1.in"
	_, err := storage.Put(root, key, strings.NewReader("old"), 8)
	require.NoError(t, err)

	_, err = storage.Put(root, key, strings.NewReader("this is way too big"), 4)
	assert.ErrorIs(t, err, storage.ErrTooLarge)

	rc, err := storage.Open(root, key)
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "old", string(got), "a too-large write must not clobber the existing object")
}

func TestRemoveAll(t *testing.T) {
	root := t.TempDir()
	_, err := storage.Put(root, "problems/1/generated/9/1.in", strings.NewReader("in"), 4)
	require.NoError(t, err)
	_, err = storage.Put(root, "problems/1/generated/9/1.out", strings.NewReader("out"), 4)
	require.NoError(t, err)
	// An unrelated object that must survive.
	_, err = storage.Put(root, "problems/1/other.in", strings.NewReader("keep"), 8)
	require.NoError(t, err)

	require.NoError(t, storage.RemoveAll(root, "problems/1/generated/9"))

	_, err = os.Stat(filepath.Join(root, "problems", "1", "generated", "9"))
	assert.True(t, os.IsNotExist(err), "generated directory must be removed")
	_, err = os.Stat(filepath.Join(root, "problems", "1", "other.in"))
	assert.NoError(t, err, "unrelated files must remain")
}

func TestRemoveAllRejectsUnsafeKeys(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"", "/etc/passwd", "../secret", "a/../../b", `a\b`, "a//b", "./a", "."} {
		assert.ErrorIs(t, storage.RemoveAll(root, key), storage.ErrInvalidKey, key)
	}
}
