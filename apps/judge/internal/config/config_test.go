package config_test

import (
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

func TestLoadRejectsBadConcurrency(t *testing.T) {
	for _, value := range []string{"0", "17", "abc"} {
		t.Setenv("KAPOJ_DATABASE_URL", "postgres://judge")
		t.Setenv("KAPOJ_JUDGE_CONCURRENCY", value)
		_, err := config.Load()
		assert.Error(t, err, value)
	}
}
