package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
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
	AIEncryptionKey    string
	AIKeyFile          string
	AIRequestTimeout   time.Duration
	AICooldown         time.Duration
	MailEncryptionKey  string
	MailKeyFile        string
	MailTimeout        time.Duration
	TrustedProxies     []string
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
	v.SetDefault("ai_encryption_key", "")
	v.SetDefault("ai_key_file", "data/ai.key")
	v.SetDefault("ai_request_timeout", "45s")
	v.SetDefault("ai_cooldown", "2s")
	v.SetDefault("mail_encryption_key", "")
	v.SetDefault("mail_key_file", "data/mail.key")
	v.SetDefault("mail_timeout", "20s")
	v.SetDefault("trusted_proxies", "")

	trustedProxies, err := parseTrustedProxies(v.GetString("trusted_proxies"))
	if err != nil {
		return nil, err
	}

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
		AIEncryptionKey:    v.GetString("ai_encryption_key"),
		AIKeyFile:          v.GetString("ai_key_file"),
		AIRequestTimeout:   v.GetDuration("ai_request_timeout"),
		AICooldown:         v.GetDuration("ai_cooldown"),
		MailEncryptionKey:  v.GetString("mail_encryption_key"),
		MailKeyFile:        v.GetString("mail_key_file"),
		MailTimeout:        v.GetDuration("mail_timeout"),
		TrustedProxies:     trustedProxies,
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
	if cfg.AIRequestTimeout <= 0 {
		return nil, errors.New("KAPOJ_AI_REQUEST_TIMEOUT must be a positive duration")
	}
	if cfg.AICooldown <= 0 {
		return nil, errors.New("KAPOJ_AI_COOLDOWN must be a positive duration")
	}
	if cfg.AIEncryptionKey != "" {
		if err := validateEncryptionKey("KAPOJ_AI_ENCRYPTION_KEY", cfg.AIEncryptionKey); err != nil {
			return nil, err
		}
	}
	if cfg.MailEncryptionKey != "" {
		if err := validateEncryptionKey("KAPOJ_MAIL_ENCRYPTION_KEY", cfg.MailEncryptionKey); err != nil {
			return nil, err
		}
	}
	if cfg.MailTimeout <= 0 {
		return nil, errors.New("KAPOJ_MAIL_TIMEOUT must be a positive duration")
	}
	return cfg, nil
}

// validateEncryptionKey 要求非空密钥为合法 base64 且解码后为 32 字节。
func validateEncryptionKey(name, key string) error {
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return fmt.Errorf("%s is not valid base64: %w", name, err)
	}
	if len(decoded) != 32 {
		return fmt.Errorf("%s must decode to 32 bytes, got %d", name, len(decoded))
	}
	return nil
}

func parseTrustedProxies(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(value); err != nil && net.ParseIP(value) == nil {
			return nil, fmt.Errorf("KAPOJ_TRUSTED_PROXIES contains invalid IP/CIDR %q", value)
		}
		out = append(out, value)
	}
	return out, nil
}
