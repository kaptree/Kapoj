package secret_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/secret"
)

func TestEnsureKeyGeneratesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail.key")
	km, err := secret.NewKeyManager("", path)
	require.NoError(t, err)
	assert.False(t, km.Configured())
	assert.Equal(t, "none", km.Source())

	require.NoError(t, km.EnsureKey())
	assert.True(t, km.Configured())
	assert.Equal(t, "file", km.Source())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(raw))
	require.NoError(t, err)
	assert.Len(t, decoded, 32)

	ct, err := km.Encrypt("x")
	require.NoError(t, err)
	got, err := km.Decrypt(ct)
	require.NoError(t, err)
	assert.Equal(t, "x", got)
}

func TestKeyManagerEnvIsReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail.key")
	km, err := secret.NewKeyManager(testKeyB64, path)
	require.NoError(t, err)
	assert.True(t, km.Configured())
	assert.Equal(t, "env", km.Source())

	require.NoError(t, km.EnsureKey())
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "env key must not generate a file")
}

func TestKeyManagerLoadsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail.key")
	require.NoError(t, os.WriteFile(path, []byte(testKeyB64+"\n"), 0o600))

	km, err := secret.NewKeyManager("", path)
	require.NoError(t, err)
	assert.True(t, km.Configured())
	assert.Equal(t, "file", km.Source())

	ct, err := km.Encrypt("value")
	require.NoError(t, err)
	got, err := km.Decrypt(ct)
	require.NoError(t, err)
	assert.Equal(t, "value", got)
}

func TestKeyManagerInvalidFileIsNone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail.key")
	require.NoError(t, os.WriteFile(path, []byte("not-a-key"), 0o600))

	km, err := secret.NewKeyManager("", path)
	require.NoError(t, err)
	assert.False(t, km.Configured())
	assert.Equal(t, "none", km.Source())
}

func TestKeyManagerRejectsInvalidEnvKey(t *testing.T) {
	_, err := secret.NewKeyManager("not-base64", filepath.Join(t.TempDir(), "mail.key"))
	assert.Error(t, err)
}

func TestKeyManagerEncryptWithoutKeyFails(t *testing.T) {
	km, err := secret.NewKeyManager("", filepath.Join(t.TempDir(), "mail.key"))
	require.NoError(t, err)
	_, err = km.Encrypt("value")
	assert.Error(t, err)
	_, err = km.Decrypt("v1:abc")
	assert.Error(t, err)
}
