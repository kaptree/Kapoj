package ai_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// newKeySettingsService 构造绑定到指定密钥文件的 SettingsService。
// envKey 非空时 KeyManager 以 env 来源加载（优先级高于文件）。
func newKeySettingsService(t *testing.T, g *gorm.DB, envKey, filePath string) *ai.SettingsService {
	t.Helper()
	km, err := ai.NewKeyManager(envKey, filePath)
	require.NoError(t, err)
	return ai.NewSettingsService(g, km, filePath)
}

// sha256Hex 返回 base64 密钥串的 SHA-256 指纹，与 KeyManager.Fingerprint 一致。
func sha256Hex(b64 string) string {
	sum := sha256.Sum256([]byte(b64))
	return hex.EncodeToString(sum[:])
}

func TestKeyGenerateAndFilePermissions(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	// 嵌套目录用于校验父目录以 0700 创建。
	file := filepath.Join(t.TempDir(), "nested", "ai.key")

	svc := newKeySettingsService(t, g, "", file)
	assert.Equal(t, "none", svc.KeyView().Source)
	assert.False(t, svc.KeyView().Configured)

	res, err := svc.GenerateKey(ctx, 42, nil)
	require.NoError(t, err)

	// 生成的密钥是 32 字节的 base64 编码。
	raw, err := base64.StdEncoding.DecodeString(res.Key)
	require.NoError(t, err)
	assert.Len(t, raw, 32)

	// 文件 0600、父目录 0700、内容等于密钥（允许末尾换行）。
	info, err := os.Stat(file)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, res.Key, strings.TrimSpace(string(data)))
	parentInfo, err := os.Stat(filepath.Dir(file))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), parentInfo.Mode().Perm())

	// KeyManager 已切换到文件来源的密钥。
	view := svc.KeyView()
	assert.True(t, view.Configured)
	assert.Equal(t, "file", view.Source)
	assert.NotEmpty(t, view.Masked)
	assert.Equal(t, sha256Hex(res.Key), view.Fingerprint)

	// 密钥只一次性返回：KeyView 的 JSON 不含 key 字段。
	viewJSON, err := json.Marshal(svc.KeyView())
	require.NoError(t, err)
	assert.NotContains(t, string(viewJSON), `"key"`)

	// 指纹写入 ai_settings.key_fingerprint。
	var s ai.Settings
	require.NoError(t, g.First(&s).Error)
	assert.Equal(t, sha256Hex(res.Key), s.KeyFingerprint)
}

func TestKeyManualValidation(t *testing.T) {
	t.Run("合法 base64/32 保存", func(t *testing.T) {
		g := testutil.PG(t)
		file := filepath.Join(t.TempDir(), "ai.key")
		svc := newKeySettingsService(t, g, "", file)

		key := b64Key(strings.Repeat("m", 32))
		view, err := svc.SetKey(context.Background(), 42, key, nil)
		require.NoError(t, err)
		assert.True(t, view.Configured)
		assert.Equal(t, "file", view.Source)
		assert.Equal(t, sha256Hex(key), view.Fingerprint)
	})

	t.Run("非法 base64 -> 40001", func(t *testing.T) {
		g := testutil.PG(t)
		svc := newKeySettingsService(t, g, "", filepath.Join(t.TempDir(), "ai.key"))
		_, err := svc.SetKey(context.Background(), 42, "not-base64-!!", nil)
		assertValidationCode(t, err, 40001)
	})

	t.Run("base64 但非 32 字节 -> 40001", func(t *testing.T) {
		g := testutil.PG(t)
		svc := newKeySettingsService(t, g, "", filepath.Join(t.TempDir(), "ai.key"))
		short := base64.StdEncoding.EncodeToString([]byte("too-short"))
		_, err := svc.SetKey(context.Background(), 42, short, nil)
		assertValidationCode(t, err, 40001)
	})

	t.Run("env 来源只读 -> 40001", func(t *testing.T) {
		g := testutil.PG(t)
		file := filepath.Join(t.TempDir(), "ai.key")
		svc := newKeySettingsService(t, g, b64Key(strings.Repeat("e", 32)), file)

		_, err := svc.SetKey(context.Background(), 42, b64Key(strings.Repeat("m", 32)), nil)
		assertValidationCode(t, err, 40001)
		_, err = svc.GenerateKey(context.Background(), 42, nil)
		assertValidationCode(t, err, 40001)
	})
}

func TestKeyRotationReencryptsUserConfigs(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "ai.key")

	oldKey := b64Key(strings.Repeat("o", 32))
	require.NoError(t, os.WriteFile(file, []byte(oldKey+"\n"), 0o600))
	svc := newKeySettingsService(t, g, "", file)
	assert.Equal(t, "file", svc.KeyView().Source)

	oldEnc, err := ai.NewEncryptor(oldKey)
	require.NoError(t, err)
	ct1, err := oldEnc.Encrypt("sk-user-1-secret")
	require.NoError(t, err)
	ct2, err := oldEnc.Encrypt("sk-user-2-secret")
	require.NoError(t, err)
	require.NoError(t, g.Create(&ai.UserConfig{
		UserID: 1, Provider: "deepseek", ModelName: "m",
		APIKeyEncrypted: ct1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)
	require.NoError(t, g.Create(&ai.UserConfig{
		UserID: 2, Provider: "openai", ModelName: "m",
		APIKeyEncrypted: ct2, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	res, err := svc.GenerateKey(ctx, 42, nil)
	require.NoError(t, err)
	newKey := res.Key

	// 新密钥能解密所有配置。
	newEnc, err := ai.NewEncryptor(newKey)
	require.NoError(t, err)
	var cfgs []ai.UserConfig
	require.NoError(t, g.Order("user_id").Find(&cfgs).Error)
	require.Len(t, cfgs, 2)
	plain1, err := newEnc.Decrypt(cfgs[0].APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "sk-user-1-secret", plain1)
	plain2, err := newEnc.Decrypt(cfgs[1].APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "sk-user-2-secret", plain2)

	// 旧密钥不再可用。
	_, err = oldEnc.Decrypt(cfgs[0].APIKeyEncrypted)
	assert.Error(t, err)

	// 备份文件已删除。
	assert.NoFileExists(t, file+".bak")

	// 文件内容为新密钥。
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, newKey, strings.TrimSpace(string(data)))

	// KeyManager 已切换到新密钥。
	view := svc.KeyView()
	assert.True(t, view.Configured)
	assert.Equal(t, "file", view.Source)
	assert.Equal(t, sha256Hex(newKey), view.Fingerprint)

	// 指纹已持久化。
	var s ai.Settings
	require.NoError(t, g.First(&s).Error)
	assert.Equal(t, sha256Hex(newKey), s.KeyFingerprint)
}

func TestKeyRotationRollback(t *testing.T) {
	t.Run("解密失败回滚", func(t *testing.T) {
		g := testutil.PG(t)
		ctx := context.Background()
		file := filepath.Join(t.TempDir(), "ai.key")
		oldKey := b64Key(strings.Repeat("o", 32))
		require.NoError(t, os.WriteFile(file, []byte(oldKey+"\n"), 0o600))
		svc := newKeySettingsService(t, g, "", file)

		oldEnc, _ := ai.NewEncryptor(oldKey)
		ct, _ := oldEnc.Encrypt("ok")
		require.NoError(t, g.Create(&ai.UserConfig{
			UserID: 1, Provider: "p", ModelName: "m",
			APIKeyEncrypted: ct, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}).Error)
		// 一条无法用旧密钥解密的密文触发解密失败。
		require.NoError(t, g.Create(&ai.UserConfig{
			UserID: 2, Provider: "p", ModelName: "m",
			APIKeyEncrypted: "v1:AAAA", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}).Error)

		_, err := svc.SetKey(ctx, 42, b64Key(strings.Repeat("n", 32)), nil)
		require.Error(t, err)

		// 旧文件与内存密钥完好。
		data, _ := os.ReadFile(file)
		assert.Equal(t, oldKey, strings.TrimSpace(string(data)))
		assert.NoFileExists(t, file+".bak")
		assert.Equal(t, sha256Hex(oldKey), svc.KeyView().Fingerprint)

		// 配置密文保持不变。
		var cfgs []ai.UserConfig
		require.NoError(t, g.Order("user_id").Find(&cfgs).Error)
		require.Len(t, cfgs, 2)
		assert.Equal(t, ct, cfgs[0].APIKeyEncrypted)
		assert.Equal(t, "v1:AAAA", cfgs[1].APIKeyEncrypted)
	})

	t.Run("事务/审计失败恢复文件", func(t *testing.T) {
		g := testutil.PG(t)
		ctx := context.Background()
		file := filepath.Join(t.TempDir(), "ai.key")
		oldKey := b64Key(strings.Repeat("o", 32))
		require.NoError(t, os.WriteFile(file, []byte(oldKey+"\n"), 0o600))
		svc := newKeySettingsService(t, g, "", file)

		oldEnc, _ := ai.NewEncryptor(oldKey)
		ct, _ := oldEnc.Encrypt("sk-keep")
		require.NoError(t, g.Create(&ai.UserConfig{
			UserID: 1, Provider: "p", ModelName: "m",
			APIKeyEncrypted: ct, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}).Error)

		_, err := svc.SetKey(ctx, 42, b64Key(strings.Repeat("n", 32)), func(tx *gorm.DB) error {
			return errors.New("audit boom")
		})
		require.Error(t, err)

		// 文件已恢复为旧密钥，备份不存在。
		data, _ := os.ReadFile(file)
		assert.Equal(t, oldKey, strings.TrimSpace(string(data)))
		assert.NoFileExists(t, file+".bak")

		// 配置不变，旧密钥仍可解密。
		var cfg ai.UserConfig
		require.NoError(t, g.First(&cfg).Error)
		assert.Equal(t, ct, cfg.APIKeyEncrypted)
		plain, err := oldEnc.Decrypt(cfg.APIKeyEncrypted)
		require.NoError(t, err)
		assert.Equal(t, "sk-keep", plain)

		// 内存密钥仍为旧密钥。
		assert.Equal(t, sha256Hex(oldKey), svc.KeyView().Fingerprint)
	})

	t.Run("文件写入失败", func(t *testing.T) {
		g := testutil.PG(t)
		ctx := context.Background()
		file := filepath.Join(t.TempDir(), "ai.key")
		oldKey := b64Key(strings.Repeat("o", 32))
		require.NoError(t, os.WriteFile(file, []byte(oldKey+"\n"), 0o600))
		// 用 .bak 目录阻断备份 rename，使文件写入失败。
		require.NoError(t, os.Mkdir(file+".bak", 0o700))
		svc := newKeySettingsService(t, g, "", file)

		oldEnc, _ := ai.NewEncryptor(oldKey)
		ct, _ := oldEnc.Encrypt("sk-keep")
		require.NoError(t, g.Create(&ai.UserConfig{
			UserID: 1, Provider: "p", ModelName: "m",
			APIKeyEncrypted: ct, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}).Error)

		_, err := svc.SetKey(ctx, 42, b64Key(strings.Repeat("n", 32)), nil)
		require.Error(t, err)

		// 旧文件与配置完好，内存密钥不变。
		data, _ := os.ReadFile(file)
		assert.Equal(t, oldKey, strings.TrimSpace(string(data)))
		var cfg ai.UserConfig
		require.NoError(t, g.First(&cfg).Error)
		assert.Equal(t, ct, cfg.APIKeyEncrypted)
		assert.Equal(t, sha256Hex(oldKey), svc.KeyView().Fingerprint)
	})
}

func TestKeyRotationWithoutOldKeyRejects(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "ai.key")
	svc := newKeySettingsService(t, g, "", file)
	assert.Equal(t, "none", svc.KeyView().Source)

	// 存在用户配置但无密钥：拒绝覆盖，返回 40001。
	require.NoError(t, g.Create(&ai.UserConfig{
		UserID: 1, Provider: "p", ModelName: "m",
		APIKeyEncrypted: "v1:AAAA", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	_, err := svc.SetKey(ctx, 42, b64Key(strings.Repeat("n", 32)), nil)
	assertValidationCode(t, err, 40001)

	// 不写文件，内存密钥仍为 none。
	assert.NoFileExists(t, file)
	assert.Equal(t, "none", svc.KeyView().Source)
}
