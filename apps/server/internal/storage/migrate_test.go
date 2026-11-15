package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateLocalToStoreSkipsSecretsAndBackups(t *testing.T) {
	source := t.TempDir()
	write := func(key, content string) {
		full := filepath.Join(source, filepath.FromSlash(key))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	write("badges/1", "badge")
	write("avatars/33/hash", "avatar")
	write("ai.key", "secret")
	write("mail.key", "secret")
	write("backups/platform.zip", "backup")
	write("custom/secret", "secret")
	write(".gitkeep", "")

	dst, err := NewLocal(t.TempDir())
	require.NoError(t, err)
	result, err := MigrateLocalToStore(context.Background(), source, dst)
	require.NoError(t, err)
	assert.EqualValues(t, 2, result.Copied)
	assert.EqualValues(t, 5, result.Skipped)

	objects, err := dst.List(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, []string{"avatars/33/hash", "badges/1"}, objectKeys(objects))
}

func objectKeys(objects []ObjectInfo) []string {
	keys := make([]string, 0, len(objects))
	for _, object := range objects {
		keys = append(keys, object.Key)
	}
	return keys
}
