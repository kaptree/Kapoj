package storage_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/storage"
)

func TestOpenReadsInsideRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "problems", "1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "problems", "1", "a.in"), []byte("1 2"), 0o644))

	rc, err := storage.Open(root, "problems/1/a.in")
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "1 2", string(b))
}

func TestOpenRejectsUnsafeKeys(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"", "/etc/passwd", "../secret", "a/../../b", `a\b`, "a//b", "./a", "."} {
		_, err := storage.Open(root, key)
		assert.ErrorIs(t, err, storage.ErrInvalidKey, key)
	}
}

func TestOpenMissingFile(t *testing.T) {
	_, err := storage.Open(t.TempDir(), "problems/1/missing.in")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}
