package password

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestHydroHashMatchesHydroOJVectors(t *testing.T) {
	cases := []struct {
		password string
		salt     string
		want     string
	}{
		{
			password: "test123",
			salt:     "abc",
			want:     "2d24d16b0954f655315984f33f1d03eb8fe888fcb4f162e7708a6861df7f2517",
		},
		{
			password: "HelloWorld",
			salt:     "aB3xY9mKpQ2rStUvWxYz0123456789AB",
			want:     "c519b45b4e118f223b8edfa027e6b07a3446fa506e634ff3f71608fb6abf5d17",
		},
	}
	for _, c := range cases {
		got := HydroHash(c.password, c.salt)
		assert.Equal(t, c.want, got, "password=%q salt=%q", c.password, c.salt)
		assert.Len(t, got, 64)
	}
}

func TestHydroHashIsDeterministic(t *testing.T) {
	assert.Equal(t, HydroHash("secret", "salt"), HydroHash("secret", "salt"))
	assert.NotEqual(t, HydroHash("secret", "salt"), HydroHash("secret", "other"))
}

func TestVerify(t *testing.T) {
	const password = "test123"
	const salt = "abc"
	hash := HydroHash(password, salt)

	assert.True(t, Verify(password, salt, hash))
	assert.False(t, Verify("wrong-password", salt, hash), "wrong password must fail")
	assert.False(t, Verify(password, "wrong-salt", hash), "wrong salt must fail")
	assert.False(t, Verify(password, "", ""), "empty stored hash must fail")
}

func TestGenerateSalt(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		salt, err := GenerateSalt()
		require.NoError(t, err)
		require.Len(t, salt, SaltLength)
		for _, r := range salt {
			assert.Contains(t, saltAlphabet, string(r))
		}
		_, duplicate := seen[salt]
		assert.False(t, duplicate, "generated salts must not repeat")
		seen[salt] = struct{}{}
	}
}

func TestHashReturnsSaltAndVerifiableHash(t *testing.T) {
	hash, salt, err := Hash("correct-horse")
	require.NoError(t, err)
	assert.Len(t, hash, 64)
	assert.Len(t, salt, SaltLength)
	assert.True(t, Verify("correct-horse", salt, hash))
	assert.False(t, Verify("wrong-horse", salt, hash))
}

func TestVerifyLegacyBcrypt(t *testing.T) {
	legacy, err := bcrypt.GenerateFromPassword([]byte("legacy-password"), bcrypt.MinCost)
	require.NoError(t, err)

	assert.True(t, IsLegacyBcrypt(string(legacy)))
	assert.True(t, Verify("legacy-password", "", string(legacy)))
	assert.False(t, Verify("wrong-password", "", string(legacy)))
	assert.False(t, IsLegacyBcrypt(strings.Repeat("a", 64)))
}
