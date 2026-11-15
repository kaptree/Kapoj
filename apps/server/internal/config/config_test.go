package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
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

func TestLoadRejectsMinIOWithoutCredentials(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	t.Setenv("KAPOJ_STORAGE_BACKEND", "minio")
	t.Setenv("KAPOJ_MINIO_ENDPOINT", "minio:9000")
	t.Setenv("KAPOJ_MINIO_ACCESS_KEY", "")
	t.Setenv("KAPOJ_MINIO_SECRET_KEY", "")
	_, err := Load()
	assert.Error(t, err)
}

func TestLoadReadsMinIOEnv(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	t.Setenv("KAPOJ_STORAGE_BACKEND", "minio")
	t.Setenv("KAPOJ_MINIO_ENDPOINT", "minio:9000")
	t.Setenv("KAPOJ_MINIO_ACCESS_KEY", "access")
	t.Setenv("KAPOJ_MINIO_SECRET_KEY", "secret")
	t.Setenv("KAPOJ_MINIO_BUCKET", "objects")
	t.Setenv("KAPOJ_MINIO_BACKUP_BUCKET", "backups")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "minio", cfg.StorageBackend)
	assert.Equal(t, "objects", cfg.MinIOBucket)
	assert.Equal(t, "backups", cfg.MinIOBackupBucket)
}

func TestLoadUnifiedConfigFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".env")
	content := strings.Join([]string{
		"KAPOJ_JWT_SECRET=" + strings.Repeat("j", 32),
		"POSTGRES_USER=dbuser",
		"POSTGRES_PASSWORD=dbpass",
		"POSTGRES_DB=kapoj",
		"KAPOJ_DB_HOST=postgres",
		"KAPOJ_DB_PORT=5433",
		"MINIO_ROOT_USER=minioadmin",
		"MINIO_ROOT_PASSWORD=miniopass",
		"KAPOJ_STORAGE_BACKEND=minio",
		"KAPOJ_MINIO_ENDPOINT=minio:9000",
		"KAPOJ_MINIO_BUCKET=objects",
		"KAPOJ_MINIO_BACKUP_BUCKET=backups",
		"KAPOJ_AI_ENCRYPTION_KEY=" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))),
	}, "\n")
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0o600))

	t.Setenv("KAPOJ_CONFIG_FILE", configPath)
	for _, key := range []string{
		"KAPOJ_JWT_SECRET", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB",
		"KAPOJ_DB_HOST", "KAPOJ_DB_PORT", "KAPOJ_DATABASE_URL",
		"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD",
		"KAPOJ_MINIO_ACCESS_KEY", "KAPOJ_MINIO_SECRET_KEY",
		"KAPOJ_STORAGE_BACKEND", "KAPOJ_MINIO_ENDPOINT",
		"KAPOJ_MINIO_BUCKET", "KAPOJ_MINIO_BACKUP_BUCKET",
		"KAPOJ_AI_ENCRYPTION_KEY",
	} {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "postgres://dbuser:dbpass@postgres:5433/kapoj?sslmode=disable", cfg.DatabaseURL)
	assert.Equal(t, "minio", cfg.StorageBackend)
	assert.Equal(t, "minioadmin", cfg.MinIOAccessKey)
	assert.Equal(t, "miniopass", cfg.MinIOSecretKey)
	assert.NotEmpty(t, cfg.AIEncryptionKey)
}

func TestLoadGameConcurrency(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 2, cfg.GameConcurrency)

	t.Setenv("KAPOJ_GAME_CONCURRENCY", "4")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, 4, cfg.GameConcurrency)

	for _, raw := range []string{"0", "-1", "17", "many"} {
		t.Setenv("KAPOJ_GAME_CONCURRENCY", raw)
		_, err = Load()
		assert.Error(t, err, "KAPOJ_GAME_CONCURRENCY=%q must be rejected", raw)
	}
}

func TestMaxEquippedBadgesConfig(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 3, cfg.MaxEquippedBadges)

	for _, raw := range []string{"1", "5", "10"} {
		t.Setenv("KAPOJ_MAX_EQUIPPED_BADGES", raw)
		cfg, err = Load()
		require.NoError(t, err, "KAPOJ_MAX_EQUIPPED_BADGES=%q 应合法", raw)
		want, convErr := strconv.Atoi(raw)
		require.NoError(t, convErr)
		assert.Equal(t, want, cfg.MaxEquippedBadges)
	}

	for _, raw := range []string{"0", "-1", "11", "many"} {
		t.Setenv("KAPOJ_MAX_EQUIPPED_BADGES", raw)
		_, err = Load()
		assert.Error(t, err, "KAPOJ_MAX_EQUIPPED_BADGES=%q 必须被拒绝", raw)
	}
}

func TestRatingSettlementConfig(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 60*time.Second, cfg.RatingScanInterval)
	assert.Equal(t, 10*time.Minute, cfg.RatingJudgeGrace)

	t.Setenv("KAPOJ_RATING_SCAN_INTERVAL", "10s")
	t.Setenv("KAPOJ_RATING_JUDGE_GRACE", "1m")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, cfg.RatingScanInterval)
	assert.Equal(t, time.Minute, cfg.RatingJudgeGrace)

	t.Setenv("KAPOJ_RATING_SCAN_INTERVAL", "10m")
	t.Setenv("KAPOJ_RATING_JUDGE_GRACE", "60m")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, 10*time.Minute, cfg.RatingScanInterval)
	assert.Equal(t, time.Hour, cfg.RatingJudgeGrace)

	cases := []struct {
		name  string
		scan  string
		grace string
	}{
		{"scan too short", "9s", "10m"},
		{"scan too long", "11m", "10m"},
		{"scan invalid", "soon", "10m"},
		{"grace too short", "60s", "59s"},
		{"grace too long", "60s", "61m"},
		{"grace invalid", "60s", "later"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("KAPOJ_RATING_SCAN_INTERVAL", c.scan)
			t.Setenv("KAPOJ_RATING_JUDGE_GRACE", c.grace)
			_, err := Load()
			assert.Error(t, err)
		})
	}
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

func TestLoadAIConfig(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	cfg, err := Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.AIEncryptionKey)
	assert.Equal(t, 45*time.Second, cfg.AIRequestTimeout)
	assert.Equal(t, 2*time.Second, cfg.AICooldown)

	t.Setenv("KAPOJ_AI_REQUEST_TIMEOUT", "10s")
	t.Setenv("KAPOJ_AI_COOLDOWN", "1s")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, cfg.AIRequestTimeout)
	assert.Equal(t, time.Second, cfg.AICooldown)

	// 合法 base64 32 字节密钥。
	validKey := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	t.Setenv("KAPOJ_AI_ENCRYPTION_KEY", validKey)
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, validKey, cfg.AIEncryptionKey)

	for _, raw := range []string{"not-base64-!!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		t.Setenv("KAPOJ_AI_ENCRYPTION_KEY", raw)
		_, err = Load()
		assert.Error(t, err, "KAPOJ_AI_ENCRYPTION_KEY=%q must be rejected", raw)
	}
}

func TestLoadAIKeyFile(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "data/ai.key", cfg.AIKeyFile)

	t.Setenv("KAPOJ_AI_KEY_FILE", "/var/lib/kapoj/ai.key")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, "/var/lib/kapoj/ai.key", cfg.AIKeyFile)
}

func TestMailConfig(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	t.Setenv("KAPOJ_MAIL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("KAPOJ_MAIL_KEY_FILE", "/tmp/mail.key")
	t.Setenv("KAPOJ_MAIL_TIMEOUT", "10s")
	t.Setenv("KAPOJ_TRUSTED_PROXIES", "127.0.0.1/32, 10.0.0.0/8")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/mail.key", cfg.MailKeyFile)
	assert.Equal(t, 10*time.Second, cfg.MailTimeout)
	assert.Equal(t, []string{"127.0.0.1/32", "10.0.0.0/8"}, cfg.TrustedProxies)
}

func TestMailConfigDefaults(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "data/mail.key", cfg.MailKeyFile)
	assert.Equal(t, 20*time.Second, cfg.MailTimeout)
	assert.Empty(t, cfg.TrustedProxies)
}

func TestMailConfigRejectsInvalidValues(t *testing.T) {
	t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
	t.Setenv("KAPOJ_MAIL_ENCRYPTION_KEY", "not-base64")
	_, err := Load()
	assert.Error(t, err)

	t.Setenv("KAPOJ_MAIL_ENCRYPTION_KEY", "")
	t.Setenv("KAPOJ_MAIL_TIMEOUT", "0s")
	_, err = Load()
	assert.Error(t, err)

	t.Setenv("KAPOJ_MAIL_TIMEOUT", "20s")
	t.Setenv("KAPOJ_TRUSTED_PROXIES", "not-a-cidr")
	_, err = Load()
	assert.Error(t, err)
}
