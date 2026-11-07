package ai_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
)

// testKeyRaw 是 32 字节的加密密钥明文；testKeyB64 是其 base64 编码。
const testKeyRaw = "0123456789abcdef0123456789abcdef"

var testKeyB64 = base64.StdEncoding.EncodeToString([]byte(testKeyRaw))

func TestEncryptorRoundTripAndWrongKey(t *testing.T) {
	enc, err := ai.NewEncryptor(testKeyB64)
	require.NoError(t, err)

	secret := "sk-0123456789abcdef"
	ct, err := enc.Encrypt(secret)
	require.NoError(t, err)
	assert.NotEqual(t, secret, ct)
	assert.True(t, strings.HasPrefix(ct, "v1:"), "ciphertext must carry a version prefix")

	got, err := enc.Decrypt(ct)
	require.NoError(t, err)
	assert.Equal(t, secret, got)

	// A different 32-byte key must fail to decrypt (GCM authentication).
	otherKey := base64.StdEncoding.EncodeToString([]byte("fedcba9876543210fedcba9876543210"))
	other, err := ai.NewEncryptor(otherKey)
	require.NoError(t, err)
	_, err = other.Decrypt(ct)
	assert.Error(t, err, "wrong key must fail decryption")
}

func TestNewEncryptorRejectsInvalidKey(t *testing.T) {
	if _, err := ai.NewEncryptor("not-valid-base64-!!"); err == nil {
		t.Fatal("invalid base64 key must be rejected")
	}
	if _, err := ai.NewEncryptor(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("key that does not decode to 32 bytes must be rejected")
	}
}
