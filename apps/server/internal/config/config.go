package config

import (
	"errors"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	RedisAddr       string
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
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
	v.SetDefault("jwt_secret", "")
	v.SetDefault("access_token_ttl", "15m")
	v.SetDefault("refresh_token_ttl", "168h")

	cfg := &Config{
		HTTPAddr:        v.GetString("http_addr"),
		DatabaseURL:     v.GetString("database_url"),
		RedisAddr:       v.GetString("redis_addr"),
		JWTSecret:       v.GetString("jwt_secret"),
		AccessTokenTTL:  v.GetDuration("access_token_ttl"),
		RefreshTokenTTL: v.GetDuration("refresh_token_ttl"),
	}

	if len(cfg.JWTSecret) < minSecretLen {
		return nil, errors.New("KAPOJ_JWT_SECRET must be at least 32 bytes")
	}
	if cfg.AccessTokenTTL <= 0 || cfg.RefreshTokenTTL <= 0 {
		return nil, errors.New("token TTLs must be positive durations")
	}
	return cfg, nil
}
