package ai_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func TestReconcileKeyFile(t *testing.T) {
	t.Run("匹配则通过", func(t *testing.T) {
		g := testutil.PG(t)
		dir := t.TempDir()
		file := filepath.Join(dir, "ai.key")
		key := b64Key(strings.Repeat("a", 32))
		require.NoError(t, os.WriteFile(file, []byte(key+"\n"), 0o600))
		km, err := ai.NewKeyManager("", file)
		require.NoError(t, err)
		require.NoError(t, g.Create(&ai.Settings{ID: 1, Enabled: true, KeyFingerprint: sha256Hex(key), UpdatedAt: time.Now()}).Error)

		require.NoError(t, ai.ReconcileKeyFile(context.Background(), g, km, file))
		assert.Equal(t, "file", km.Source())
		assert.Equal(t, sha256Hex(key), km.Fingerprint())
	})

	t.Run("env 来源跳过", func(t *testing.T) {
		g := testutil.PG(t)
		dir := t.TempDir()
		file := filepath.Join(dir, "ai.key")
		envKey := b64Key(strings.Repeat("e", 32))
		km, err := ai.NewKeyManager(envKey, file)
		require.NoError(t, err)
		// DB 指纹与 env 密钥不一致也不应触发任何恢复。
		require.NoError(t, g.Create(&ai.Settings{ID: 1, Enabled: true, KeyFingerprint: sha256Hex(b64Key(strings.Repeat("z", 32))), UpdatedAt: time.Now()}).Error)

		require.NoError(t, ai.ReconcileKeyFile(context.Background(), g, km, file))
		assert.Equal(t, "env", km.Source())
	})

	t.Run("数据库指纹为空跳过", func(t *testing.T) {
		g := testutil.PG(t)
		dir := t.TempDir()
		file := filepath.Join(dir, "ai.key")
		key := b64Key(strings.Repeat("a", 32))
		require.NoError(t, os.WriteFile(file, []byte(key+"\n"), 0o600))
		km, err := ai.NewKeyManager("", file)
		require.NoError(t, err)
		require.NoError(t, g.Create(&ai.Settings{ID: 1, Enabled: true, KeyFingerprint: "", UpdatedAt: time.Now()}).Error)

		require.NoError(t, ai.ReconcileKeyFile(context.Background(), g, km, file))
	})

	t.Run("无设置行跳过", func(t *testing.T) {
		g := testutil.PG(t)
		dir := t.TempDir()
		file := filepath.Join(dir, "ai.key")
		key := b64Key(strings.Repeat("a", 32))
		require.NoError(t, os.WriteFile(file, []byte(key+"\n"), 0o600))
		km, err := ai.NewKeyManager("", file)
		require.NoError(t, err)

		require.NoError(t, ai.ReconcileKeyFile(context.Background(), g, km, file))
	})

	t.Run("崩溃窗口：备份指纹匹配则恢复", func(t *testing.T) {
		g := testutil.PG(t)
		dir := t.TempDir()
		file := filepath.Join(dir, "ai.key")
		oldKey := b64Key(strings.Repeat("o", 32))
		newKey := b64Key(strings.Repeat("n", 32))
		// 模拟崩溃：文件已写入新密钥，备份是旧密钥，DB 指纹仍是旧密钥。
		require.NoError(t, os.WriteFile(file, []byte(newKey+"\n"), 0o600))
		require.NoError(t, os.WriteFile(file+".bak", []byte(oldKey+"\n"), 0o600))
		require.NoError(t, g.Create(&ai.Settings{ID: 1, Enabled: true, KeyFingerprint: sha256Hex(oldKey), UpdatedAt: time.Now()}).Error)

		// 用户配置用旧密钥加密，恢复后必须可解密。
		oldEnc, err := ai.NewEncryptor(oldKey)
		require.NoError(t, err)
		ct, err := oldEnc.Encrypt("sk-secret")
		require.NoError(t, err)
		require.NoError(t, g.Create(&ai.UserConfig{
			UserID: 1, Provider: "deepseek", ModelName: "deepseek-chat",
			APIKeyEncrypted: ct, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}).Error)

		km, err := ai.NewKeyManager("", file)
		require.NoError(t, err)
		assert.Equal(t, sha256Hex(newKey), km.Fingerprint(), "崩溃后启动读到新密钥")

		require.NoError(t, ai.ReconcileKeyFile(context.Background(), g, km, file))

		// 密钥文件恢复为旧密钥，备份消失，内存密钥切换为旧密钥。
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, oldKey, strings.TrimSpace(string(data)))
		assert.NoFileExists(t, file+".bak")
		assert.Equal(t, sha256Hex(oldKey), km.Fingerprint())
		assert.Equal(t, "file", km.Source())

		// 用户配置仍可解密。
		plain, err := km.Decrypt(ct)
		require.NoError(t, err)
		assert.Equal(t, "sk-secret", plain)
	})

	t.Run("无备份或备份不匹配则报错", func(t *testing.T) {
		g := testutil.PG(t)
		dir := t.TempDir()
		file := filepath.Join(dir, "ai.key")
		newKey := b64Key(strings.Repeat("n", 32))
		require.NoError(t, os.WriteFile(file, []byte(newKey+"\n"), 0o600))
		require.NoError(t, g.Create(&ai.Settings{ID: 1, Enabled: true, KeyFingerprint: sha256Hex(b64Key(strings.Repeat("o", 32))), UpdatedAt: time.Now()}).Error)

		km, err := ai.NewKeyManager("", file)
		require.NoError(t, err)

		err = ai.ReconcileKeyFile(context.Background(), g, km, file)
		require.Error(t, err)
		// 不静默启动：不修改文件、不切换内存密钥。
		assert.Equal(t, sha256Hex(newKey), km.Fingerprint())
		data, _ := os.ReadFile(file)
		assert.Equal(t, newKey, strings.TrimSpace(string(data)))
	})

	t.Run("密钥文件丢失但备份匹配则恢复", func(t *testing.T) {
		g := testutil.PG(t)
		dir := t.TempDir()
		file := filepath.Join(dir, "ai.key")
		oldKey := b64Key(strings.Repeat("o", 32))
		require.NoError(t, os.WriteFile(file+".bak", []byte(oldKey+"\n"), 0o600))
		require.NoError(t, g.Create(&ai.Settings{ID: 1, Enabled: true, KeyFingerprint: sha256Hex(oldKey), UpdatedAt: time.Now()}).Error)

		km, err := ai.NewKeyManager("", file) // 文件不存在 -> none
		require.NoError(t, err)
		assert.Equal(t, "none", km.Source())

		require.NoError(t, ai.ReconcileKeyFile(context.Background(), g, km, file))
		assert.Equal(t, "file", km.Source())
		assert.Equal(t, sha256Hex(oldKey), km.Fingerprint())
	})
}
