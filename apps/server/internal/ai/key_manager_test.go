package ai_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// b64Key encodes a 32-byte ASCII key as base64 for use as an encryption key.
func b64Key(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// noKeyCode asserts err is the 40002 "AI 加密密钥未配置" error.
func noKeyCode(t *testing.T, err error) {
	t.Helper()
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 40002, appErr.Code)
}

func TestKeyManagerEnvPrecedenceAndFile(t *testing.T) {
	t.Run("env valid wins over file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "ai.key")
		fileKey := b64Key(strings.Repeat("f", 32))
		require.NoError(t, os.WriteFile(filePath, []byte(fileKey+"\n"), 0o600))

		envKey := b64Key(strings.Repeat("e", 32))
		km, err := ai.NewKeyManager(envKey, filePath)
		require.NoError(t, err)
		assert.True(t, km.Configured())
		assert.Equal(t, "env", km.Source())

		// Ciphertext must decrypt with the env key, not the file key.
		ct, err := km.Encrypt("secret")
		require.NoError(t, err)
		envEnc, err := ai.NewEncryptor(envKey)
		require.NoError(t, err)
		plain, err := envEnc.Decrypt(ct)
		require.NoError(t, err)
		assert.Equal(t, "secret", plain)

		fileEnc, err := ai.NewEncryptor(fileKey)
		require.NoError(t, err)
		_, err = fileEnc.Decrypt(ct)
		assert.Error(t, err, "file key must be ignored when env key is set")
	})

	t.Run("env invalid errors", func(t *testing.T) {
		_, err := ai.NewKeyManager("not-valid-base64-!!", filepath.Join(t.TempDir(), "ai.key"))
		assert.Error(t, err, "invalid env key must fail startup")
	})
}

func TestKeyManagerFileMissingInvalid(t *testing.T) {
	t.Run("missing file -> none", func(t *testing.T) {
		km, err := ai.NewKeyManager("", filepath.Join(t.TempDir(), "nonexistent"))
		require.NoError(t, err)
		assert.False(t, km.Configured())
		assert.Equal(t, "none", km.Source())
	})

	t.Run("invalid file -> none, no panic", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "ai.key")
		require.NoError(t, os.WriteFile(filePath, []byte("garbage-not-base64"), 0o600))

		km, err := ai.NewKeyManager("", filePath)
		require.NoError(t, err)
		assert.False(t, km.Configured())
		assert.Equal(t, "none", km.Source())
		assert.Empty(t, km.Masked())
		assert.Empty(t, km.Fingerprint())
	})

	t.Run("valid file -> source file, masked/fingerprint", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "ai.key")
		fileKey := b64Key(strings.Repeat("f", 32))
		require.NoError(t, os.WriteFile(filePath, []byte(fileKey+"\n"), 0o600))

		km, err := ai.NewKeyManager("", filePath)
		require.NoError(t, err)
		assert.True(t, km.Configured())
		assert.Equal(t, "file", km.Source())

		assert.Equal(t, fileKey[:3]+"…"+fileKey[len(fileKey)-4:], km.Masked())

		sum := sha256.Sum256([]byte(fileKey))
		assert.Equal(t, hex.EncodeToString(sum[:]), km.Fingerprint())
	})

	t.Run("unreadable file -> none", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "ai.key")
		require.NoError(t, os.WriteFile(filePath, []byte(b64Key(strings.Repeat("f", 32))), 0o600))
		require.NoError(t, os.Chmod(filePath, 0o000))
		t.Cleanup(func() { _ = os.Chmod(filePath, 0o600) })

		km, err := ai.NewKeyManager("", filePath)
		require.NoError(t, err)
		assert.False(t, km.Configured())
		assert.Equal(t, "none", km.Source())
	})
}

func TestKeyManagerNoKeyEncryptDecrypt(t *testing.T) {
	km, err := ai.NewKeyManager("", filepath.Join(t.TempDir(), "nonexistent"))
	require.NoError(t, err)

	_, err = km.Encrypt("x")
	noKeyCode(t, err)
	_, err = km.Decrypt("v1:AAAA")
	noKeyCode(t, err)
}

func TestKeyManagerReplaceAndConcurrency(t *testing.T) {
	km, err := ai.NewKeyManager("", filepath.Join(t.TempDir(), "nonexistent"))
	require.NoError(t, err)
	assert.False(t, km.Configured())

	keyA := b64Key(strings.Repeat("a", 32))
	encA, err := ai.NewEncryptor(keyA)
	require.NoError(t, err)
	km.Replace(encA, "file")
	assert.True(t, km.Configured())
	assert.Equal(t, "file", km.Source())

	ct, err := km.Encrypt("hello")
	require.NoError(t, err)
	plain, err := km.Decrypt(ct)
	require.NoError(t, err)
	assert.Equal(t, "hello", plain)

	// After replacing with a new key, the old ciphertext must no longer decrypt.
	keyB := b64Key(strings.Repeat("b", 32))
	encB, err := ai.NewEncryptor(keyB)
	require.NoError(t, err)
	km.Replace(encB, "file")
	_, err = km.Decrypt(ct)
	assert.Error(t, err, "old ciphertext must fail after key replacement")

	// Concurrent Encrypt/Decrypt/read while another goroutine keeps replacing.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c, _ := km.Encrypt("x")
				_, _ = km.Decrypt(c)
				_ = km.Configured()
				_ = km.Source()
				_ = km.Masked()
				_ = km.Fingerprint()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 10; j++ {
			enc, _ := ai.NewEncryptor(b64Key(strings.Repeat("c", 32)))
			km.Replace(enc, "file")
		}
	}()
	wg.Wait()
}

func TestAIConfigWithoutKey(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	km, err := ai.NewKeyManager("", filepath.Join(t.TempDir(), "nonexistent"))
	require.NoError(t, err)
	svc := ai.NewService(g, km, nil, nil)
	assert.False(t, svc.KeyConfigured())

	// GET config -> 200 semantics: configured=false, keyConfigured=false, providers.
	view, err := svc.GetConfig(ctx, 1)
	require.NoError(t, err)
	assert.False(t, view.Configured)
	assert.False(t, view.KeyConfigured)
	assert.Len(t, view.Providers, 4)

	// SaveConfig -> 40002（平台密钥缺失）。
	_, err = svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	assertValidationCode(t, err, 40002)

	// generate -> 40001（用户未配置 AI 模型）。
	_, err = svc.Complete(ctx, 1, ai.GenerateInput{Code: "x", Language: "cpp", Cursor: intPtr(1)})
	assertValidationCode(t, err, 40001)

	// With a stored config but no platform key, generate/hint still hit the
	// no-key decrypt path and return 40002.
	require.NoError(t, g.Create(&ai.UserConfig{
		UserID: 2, Provider: "deepseek", ModelName: "deepseek-chat",
		APIKeyEncrypted: "v1:AAAA", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)
	_, err = svc.Complete(ctx, 2, ai.GenerateInput{Code: "x", Language: "cpp", Cursor: intPtr(1)})
	assertValidationCode(t, err, 40002)

	// hint: cached row is still served without a key.
	cachedPID := createProblem(t, g, "无密钥提示", true)
	require.NoError(t, g.Create(&ai.Hint{
		ProblemID: cachedPID, Content: "缓存提示", CreatedBy: 1, CreatedAt: time.Now(),
	}).Error)
	h, err := svc.Hint(ctx, 1, cachedPID, false)
	require.NoError(t, err)
	assert.True(t, h.Cached)
	assert.Equal(t, "缓存提示", h.Hint)

	// hint: cache miss with no key -> 40002.
	missPID := createProblem(t, g, "无密钥提示2", true)
	_, err = svc.Hint(ctx, 2, missPID, false)
	assertValidationCode(t, err, 40002)
}

func TestAIServiceUsesCurrentKeyAfterReplace(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	keyA := b64Key(strings.Repeat("a", 32))
	km, err := ai.NewKeyManager(keyA, filepath.Join(t.TempDir(), "nonexistent"))
	require.NoError(t, err)
	svc := ai.NewService(g, km, nil, nil)
	assert.True(t, svc.KeyConfigured())

	// Save user 1 under key A.
	_, err = svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "sk-under-A"})
	require.NoError(t, err)

	// Replace the in-memory key with B.
	keyB := b64Key(strings.Repeat("b", 32))
	encB, err := ai.NewEncryptor(keyB)
	require.NoError(t, err)
	km.Replace(encB, "file")

	// New writes must use key B.
	view, err := svc.SaveConfig(ctx, 2, ai.SaveConfigInput{Provider: "deepseek", APIKey: "sk-under-B"})
	require.NoError(t, err)
	assert.True(t, view.Configured)
	assert.True(t, view.KeyConfigured)

	var cfg ai.UserConfig
	require.NoError(t, g.Where("user_id = ?", 2).First(&cfg).Error)
	encB2, err := ai.NewEncryptor(keyB)
	require.NoError(t, err)
	plain, err := encB2.Decrypt(cfg.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "sk-under-B", plain)

	encA, err := ai.NewEncryptor(keyA)
	require.NoError(t, err)
	_, err = encA.Decrypt(cfg.APIKeyEncrypted)
	assert.Error(t, err, "new ciphertext must not decrypt with the old key A")

	// GetConfig decrypts with the current (B) key.
	got, err := svc.GetConfig(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, "sk-…er-B", got.APIKeyMasked)
}
