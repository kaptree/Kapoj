package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/subosito/gotenv"

	"github.com/kaptree/kapoj/apps/judge/internal/model"
)

type Config struct {
	DatabaseURL           string
	RedisAddr             string
	StorageDir            string
	StorageBackend        string
	MinIOEndpoint         string
	MinIOAccessKey        string
	MinIOSecretKey        string
	MinIOBucket           string
	MinIORegion           string
	MinIOUseSSL           bool
	MinIOAutoCreateBucket bool
	Concurrency           int
	DockerBin             string
	Images                map[model.Language]string
}

func loadConfigFile() error {
	path := os.Getenv("KAPOJ_CONFIG_FILE")
	if path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open config file %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	values, err := gotenv.StrictParse(file)
	if err != nil {
		return fmt.Errorf("parse config file %q: %w", path, err)
	}
	for key, value := range values {
		if key == "KAPOJ_CONFIG_FILE" {
			continue
		}
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s from config file: %w", key, err)
			}
		}
	}
	return nil
}

func databaseURLFromPostgres() string {
	user := os.Getenv("POSTGRES_USER")
	database := os.Getenv("POSTGRES_DB")
	if user == "" || database == "" {
		return ""
	}
	host := env("KAPOJ_DB_HOST", "localhost")
	port := env("KAPOJ_DB_PORT", "5432")
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, os.Getenv("POSTGRES_PASSWORD")),
		Host:     net.JoinHostPort(host, port),
		Path:     "/" + database,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

func Load() (*Config, error) {
	if err := loadConfigFile(); err != nil {
		return nil, err
	}
	databaseURL := os.Getenv("KAPOJ_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = databaseURLFromPostgres()
	}
	cfg := &Config{
		DatabaseURL:           databaseURL,
		RedisAddr:             env("KAPOJ_REDIS_ADDR", "localhost:6379"),
		StorageDir:            env("KAPOJ_STORAGE_DIR", "data"),
		StorageBackend:        env("KAPOJ_STORAGE_BACKEND", "local"),
		MinIOEndpoint:         env("KAPOJ_MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:        os.Getenv("KAPOJ_MINIO_ACCESS_KEY"),
		MinIOSecretKey:        os.Getenv("KAPOJ_MINIO_SECRET_KEY"),
		MinIOBucket:           env("KAPOJ_MINIO_BUCKET", "kapoj"),
		MinIORegion:           os.Getenv("KAPOJ_MINIO_REGION"),
		MinIOUseSSL:           envBool("KAPOJ_MINIO_USE_SSL", false),
		MinIOAutoCreateBucket: envBool("KAPOJ_MINIO_AUTO_CREATE_BUCKET", true),
		Concurrency:           2,
		DockerBin:             env("KAPOJ_JUDGE_DOCKER", "docker"),
	}
	if cfg.MinIOAccessKey == "" {
		cfg.MinIOAccessKey = os.Getenv("MINIO_ROOT_USER")
	}
	if cfg.MinIOSecretKey == "" {
		cfg.MinIOSecretKey = os.Getenv("MINIO_ROOT_PASSWORD")
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("KAPOJ_DATABASE_URL or POSTGRES_* is required")
	}
	switch cfg.StorageBackend {
	case "local":
	case "minio":
		if cfg.MinIOEndpoint == "" || cfg.MinIOBucket == "" {
			return nil, errors.New("KAPOJ_MINIO_ENDPOINT and KAPOJ_MINIO_BUCKET are required when KAPOJ_STORAGE_BACKEND=minio")
		}
		if cfg.MinIOAccessKey == "" || cfg.MinIOSecretKey == "" {
			return nil, errors.New("KAPOJ_MINIO_ACCESS_KEY and KAPOJ_MINIO_SECRET_KEY are required when KAPOJ_STORAGE_BACKEND=minio")
		}
	default:
		return nil, fmt.Errorf("KAPOJ_STORAGE_BACKEND must be local or minio, got %q", cfg.StorageBackend)
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

func envBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}
