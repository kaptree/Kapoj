package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/config"
)

func setRequired(t *testing.T) {
	t.Setenv("KAPOJ_DATABASE_URL", "postgres://judge")
}

func TestLoadRequiresDatabase(t *testing.T) {
	t.Setenv("KAPOJ_DATABASE_URL", "")
	_, err := config.Load()
	assert.Error(t, err)
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "postgres://judge", cfg.DatabaseURL)
	assert.Equal(t, "localhost:6379", cfg.RedisAddr)
	assert.Equal(t, "data", cfg.StorageDir)
	assert.Equal(t, 2, cfg.Concurrency)
	assert.Equal(t, "docker", cfg.DockerBin)
	assert.Equal(t, "gcc:14", cfg.Images["cpp"])
	assert.Equal(t, "gcc:14", cfg.Images["c"])
	assert.Equal(t, "python:3.12-slim", cfg.Images["python3"])
	assert.Equal(t, "eclipse-temurin:21-jdk", cfg.Images["java"])
	assert.Equal(t, "golang:1.26", cfg.Images["go"])
}

func TestLoadReadsEnv(t *testing.T) {
	setRequired(t)
	t.Setenv("KAPOJ_REDIS_ADDR", "redis:6379")
	t.Setenv("KAPOJ_STORAGE_DIR", "/data")
	t.Setenv("KAPOJ_JUDGE_CONCURRENCY", "4")
	t.Setenv("KAPOJ_JUDGE_DOCKER", "/usr/bin/docker")
	t.Setenv("KAPOJ_JUDGE_CPP_IMAGE", "gcc:15")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "redis:6379", cfg.RedisAddr)
	assert.Equal(t, "/data", cfg.StorageDir)
	assert.Equal(t, 4, cfg.Concurrency)
	assert.Equal(t, "/usr/bin/docker", cfg.DockerBin)
	assert.Equal(t, "gcc:15", cfg.Images["cpp"])
}

func TestLoadUnifiedConfigFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".env")
	content := strings.Join([]string{
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
	}, "\n")
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0o600))
	t.Setenv("KAPOJ_CONFIG_FILE", configPath)
	for _, key := range []string{
		"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB",
		"KAPOJ_DB_HOST", "KAPOJ_DB_PORT", "KAPOJ_DATABASE_URL",
		"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD",
		"KAPOJ_MINIO_ACCESS_KEY", "KAPOJ_MINIO_SECRET_KEY",
		"KAPOJ_STORAGE_BACKEND", "KAPOJ_MINIO_ENDPOINT", "KAPOJ_MINIO_BUCKET",
	} {
		t.Setenv(key, "")
	}

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "postgres://dbuser:dbpass@postgres:5433/kapoj?sslmode=disable", cfg.DatabaseURL)
	assert.Equal(t, "minioadmin", cfg.MinIOAccessKey)
	assert.Equal(t, "miniopass", cfg.MinIOSecretKey)
	assert.Equal(t, "objects", cfg.MinIOBucket)
}

func TestLoadRejectsBadConcurrency(t *testing.T) {
	for _, value := range []string{"0", "17", "abc"} {
		t.Setenv("KAPOJ_DATABASE_URL", "postgres://judge")
		t.Setenv("KAPOJ_JUDGE_CONCURRENCY", value)
		_, err := config.Load()
		assert.Error(t, err, value)
	}
}
