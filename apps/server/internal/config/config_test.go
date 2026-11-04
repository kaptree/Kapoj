package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRequiresJWTSecret(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", "")
	_, err := Load()
	assert.Error(t, err)

	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 31))
	_, err = Load()
	assert.Error(t, err)
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.HTTPAddr)
	assert.Equal(t, "localhost:6379", cfg.RedisAddr)
	assert.Equal(t, 15*time.Minute, cfg.AccessTokenTTL)
	assert.Equal(t, 168*time.Hour, cfg.RefreshTokenTTL)
}

func TestLoadReadsEnv(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	t.Setenv("KAPOJ_HTTP_ADDR", ":9000")
	t.Setenv("KAPOJ_DATABASE_URL", "postgres://x")
	t.Setenv("KAPOJ_ACCESS_TOKEN_TTL", "5m")
	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, ":9000", cfg.HTTPAddr)
	assert.Equal(t, "postgres://x", cfg.DatabaseURL)
	assert.Equal(t, 5*time.Minute, cfg.AccessTokenTTL)
}

func TestLoadStorageDirDefault(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "data", cfg.StorageDir)
}

func TestLoadStorageDirFromEnv(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	t.Setenv("KAPOJ_STORAGE_DIR", "/var/lib/kapoj")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/var/lib/kapoj", cfg.StorageDir)
}

func TestLoadSubmitCooldown(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 3*time.Second, cfg.SubmitCooldown)

	t.Setenv("KAPOJ_SUBMIT_COOLDOWN", "5s")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, cfg.SubmitCooldown)

	t.Setenv("KAPOJ_SUBMIT_COOLDOWN", "0s")
	_, err = Load()
	assert.Error(t, err)
}
