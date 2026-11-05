package config

import (
	"errors"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	RedisAddr          string
	StorageDir         string
	JWTSecret          string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	SubmitCooldown     time.Duration
	GameConcurrency    int
	MaxEquippedBadges  int
	RatingScanInterval time.Duration
	RatingJudgeGrace   time.Duration
}

const minSecretLen = 32

// Load reads configuration from KAPOJ_* environment variables.
func Load() (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("KAPOJ")
	v.AutomaticEnv()

	v.SetDefault("http_addr", ":8080")
	v.SetDefault("database_url", "")
	v.SetDefault("redis_addr", "localhost:6379")
	v.SetDefault("storage_dir", "data")
	v.SetDefault("jwt_secret", "")
	v.SetDefault("access_token_ttl", "15m")
	v.SetDefault("refresh_token_ttl", "168h")
	v.SetDefault("submit_cooldown", "3s")
	v.SetDefault("game_concurrency", 2)
	v.SetDefault("max_equipped_badges", 3)
	v.SetDefault("rating_scan_interval", "60s")
	v.SetDefault("rating_judge_grace", "10m")

	cfg := &Config{
		HTTPAddr:           v.GetString("http_addr"),
		DatabaseURL:        v.GetString("database_url"),
		RedisAddr:          v.GetString("redis_addr"),
		StorageDir:         v.GetString("storage_dir"),
		JWTSecret:          v.GetString("jwt_secret"),
		AccessTokenTTL:     v.GetDuration("access_token_ttl"),
		RefreshTokenTTL:    v.GetDuration("refresh_token_ttl"),
		SubmitCooldown:     v.GetDuration("submit_cooldown"),
		GameConcurrency:    v.GetInt("game_concurrency"),
		MaxEquippedBadges:  v.GetInt("max_equipped_badges"),
		RatingScanInterval: v.GetDuration("rating_scan_interval"),
		RatingJudgeGrace:   v.GetDuration("rating_judge_grace"),
	}

	if len(cfg.JWTSecret) < minSecretLen {
		return nil, errors.New("KAPOJ_JWT_SECRET must be at least 32 bytes")
	}
	if cfg.AccessTokenTTL <= 0 || cfg.RefreshTokenTTL <= 0 {
		return nil, errors.New("token TTLs must be positive durations")
	}
	if cfg.SubmitCooldown <= 0 {
		return nil, errors.New("KAPOJ_SUBMIT_COOLDOWN must be a positive duration")
	}
	if cfg.GameConcurrency < 1 || cfg.GameConcurrency > 16 {
		return nil, errors.New("KAPOJ_GAME_CONCURRENCY must be between 1 and 16")
	}
	if cfg.MaxEquippedBadges < 1 || cfg.MaxEquippedBadges > 10 {
		return nil, errors.New("KAPOJ_MAX_EQUIPPED_BADGES must be between 1 and 10")
	}
	if cfg.RatingScanInterval < 10*time.Second || cfg.RatingScanInterval > 10*time.Minute {
		return nil, errors.New("KAPOJ_RATING_SCAN_INTERVAL must be between 10s and 10m")
	}
	if cfg.RatingJudgeGrace < time.Minute || cfg.RatingJudgeGrace > time.Hour {
		return nil, errors.New("KAPOJ_RATING_JUDGE_GRACE must be between 1m and 60m")
	}
	return cfg, nil
}
