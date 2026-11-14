package backup_test

import (
	"encoding/json"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/backup"
)

func TestArchiveRoundTripAndManifest(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test.zip")
	w, err := backup.NewWriter(file, 10)
	require.NoError(t, err)
	entry, err := w.Create("files/hello.txt")
	require.NoError(t, err)
	_, err = entry.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, w.Close(backup.Manifest{Kind: backup.KindPlatform, CreatedAt: time.Now()}))

	r, err := backup.OpenReader(file, 10, 1024)
	require.NoError(t, err)
	defer r.Close()
	assert.Equal(t, backup.KindPlatform, r.Manifest.Kind)
	rc, err := r.Open("files/hello.txt")
	require.NoError(t, err)
	defer rc.Close()
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	var manifest backup.Manifest
	rc2, err := r.Open("manifest.json")
	require.NoError(t, err)
	defer rc2.Close()
	require.NoError(t, json.NewDecoder(rc2).Decode(&manifest))
	assert.Equal(t, 1, manifest.FileCount)
}

func TestArchiveRejectsTraversalAndLimits(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bad.zip")
	w, err := backup.NewWriter(file, 1)
	require.NoError(t, err)
	_, err = w.Create("../escape.txt")
	assert.Error(t, err)
	_, err = w.Create("files/one.txt")
	require.NoError(t, err)
	_, err = w.Create("files/two.txt")
	assert.Error(t, err)
	w.Abort()
}
