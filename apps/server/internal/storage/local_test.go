package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLocal(t *testing.T) (*Local, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "store")
	l, err := NewLocal(root)
	require.NoError(t, err)
	return l, root
}

func readKey(t *testing.T, s Storage, key string) string {
	t.Helper()
	rc, err := s.Open(context.Background(), key)
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	require.NoError(t, err)
	return string(b)
}

func files(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func TestPutOpenRoundTrip(t *testing.T) {
	l, _ := newLocal(t)
	n, err := l.Put(context.Background(), "a/b.txt", strings.NewReader("hello"), 100)
	require.NoError(t, err)
	assert.Equal(t, int64(5), n)
	assert.Equal(t, "hello", readKey(t, l, "a/b.txt"))
}

func TestPutCreatesNestedDirs(t *testing.T) {
	l, root := newLocal(t)
	_, err := l.Put(context.Background(), "p/1/testcases/x.in", strings.NewReader("1"), 10)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "p", "1", "testcases", "x.in"))
}

func TestPutOverwrites(t *testing.T) {
	l, _ := newLocal(t)
	ctx := context.Background()
	_, _ = l.Put(ctx, "k", strings.NewReader("old"), 10)
	_, err := l.Put(ctx, "k", strings.NewReader("new!"), 10)
	require.NoError(t, err)
	assert.Equal(t, "new!", readKey(t, l, "k"))
}

func TestPutTooLargeLeavesNothing(t *testing.T) {
	l, root := newLocal(t)
	_, err := l.Put(context.Background(), "big/k", strings.NewReader("0123456789"), 5)
	assert.ErrorIs(t, err, ErrTooLarge)
	assert.Empty(t, files(t, root), "no target or temp file may remain")
	_, err = l.Open(context.Background(), "big/k")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPutTooLargeKeepsPreviousContent(t *testing.T) {
	l, _ := newLocal(t)
	ctx := context.Background()
	_, _ = l.Put(ctx, "k", strings.NewReader("old"), 10)
	_, err := l.Put(ctx, "k", strings.NewReader("0123456789"), 5)
	assert.ErrorIs(t, err, ErrTooLarge)
	assert.Equal(t, "old", readKey(t, l, "k"))
}

func TestPutExactlyMaxBytes(t *testing.T) {
	l, _ := newLocal(t)
	n, err := l.Put(context.Background(), "k", strings.NewReader("12345"), 5)
	require.NoError(t, err)
	assert.Equal(t, int64(5), n)
}

func TestOpenMissing(t *testing.T) {
	l, _ := newLocal(t)
	_, err := l.Open(context.Background(), "nope")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteIdempotent(t *testing.T) {
	l, _ := newLocal(t)
	ctx := context.Background()
	_, _ = l.Put(ctx, "k", strings.NewReader("x"), 10)
	require.NoError(t, l.Delete(ctx, "k"))
	require.NoError(t, l.Delete(ctx, "k"))
	_, err := l.Open(ctx, "k")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestInvalidKeys(t *testing.T) {
	l, root := newLocal(t)
	ctx := context.Background()
	for _, key := range []string{"", "../x", "/abs", "a/../../b", "a\\b", "a\x00b", ".", "a/./b"} {
		_, err := l.Put(ctx, key, strings.NewReader("x"), 10)
		assert.ErrorIs(t, err, ErrInvalidKey, "put %q", key)
		_, err = l.Open(ctx, key)
		assert.ErrorIs(t, err, ErrInvalidKey, "open %q", key)
		assert.ErrorIs(t, l.Delete(ctx, key), ErrInvalidKey, "delete %q", key)
	}
	assert.Empty(t, files(t, root))
	assert.Empty(t, files(t, filepath.Dir(root)), "nothing may be written outside the root")
}
