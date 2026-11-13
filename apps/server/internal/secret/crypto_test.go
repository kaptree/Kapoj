package secret_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/secret"
)

const testKeyRaw = "0123456789abcdef0123456789abcdef"

var (
	testKeyB64  = base64.StdEncoding.EncodeToString([]byte(testKeyRaw))
	otherKeyB64 = base64.StdEncoding.EncodeToString([]byte("fedcba9876543210fedcba9876543210"))
)

func TestEncryptorRoundTripAndWrongKey(t *testing.T) {
	enc, err := secret.NewEncryptor(testKeyB64)
	require.NoError(t, err)

	ct, err := enc.Encrypt("smtp-password")
	require.NoError(t, err)
	assert.NotEqual(t, "smtp-password", ct)
	assert.True(t, strings.HasPrefix(ct, "v1:"), "ciphertext must carry a version prefix")

	got, err := enc.Decrypt(ct)
	require.NoError(t, err)
	assert.Equal(t, "smtp-password", got)

	other, err := secret.NewEncryptor(otherKeyB64)
	require.NoError(t, err)
	_, err = other.Decrypt(ct)
	assert.Error(t, err, "wrong key must fail decryption")
}

func TestNewEncryptorRejectsInvalidKey(t *testing.T) {
	if _, err := secret.NewEncryptor("not-valid-base64-!!"); err == nil {
		t.Fatal("invalid base64 key must be rejected")
	}
	if _, err := secret.NewEncryptor(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("key that does not decode to 32 bytes must be rejected")
	}
}
