package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/kaptree/kapoj/apps/judge/internal/model"
)

type Config struct {
	DatabaseURL string
	RedisAddr   string
	StorageDir  string
	Concurrency int
	DockerBin   string
	Images      map[model.Language]string
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL: os.Getenv("KAPOJ_DATABASE_URL"),
		RedisAddr:   env("KAPOJ_REDIS_ADDR", "localhost:6379"),
		StorageDir:  env("KAPOJ_STORAGE_DIR", "data"),
		Concurrency: 2,
		DockerBin:   env("KAPOJ_JUDGE_DOCKER", "docker"),
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("KAPOJ_DATABASE_URL is required")
	}
	if raw := os.Getenv("KAPOJ_JUDGE_CONCURRENCY"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 16 {
			return nil, fmt.Errorf("KAPOJ_JUDGE_CONCURRENCY must be between 1 and 16, got %q", raw)
		}
		cfg.Concurrency = n
	}
	cfg.Images = map[model.Language]string{
		model.LanguageC:       env("KAPOJ_JUDGE_CPP_IMAGE", "gcc:14"),
		model.LanguageCPP:     env("KAPOJ_JUDGE_CPP_IMAGE", "gcc:14"),
		model.LanguagePython3: env("KAPOJ_JUDGE_PYTHON_IMAGE", "python:3.12-slim"),
		model.LanguageJava:    env("KAPOJ_JUDGE_JAVA_IMAGE", "eclipse-temurin:21-jdk"),
		model.LanguageGo:      env("KAPOJ_JUDGE_GO_IMAGE", "golang:1.26"),
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
