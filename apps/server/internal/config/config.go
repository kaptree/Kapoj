package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

type Config struct {
	HTTPAddr                 string
	DatabaseURL              string
	RedisAddr                string
	StorageDir               string
	RequirePersistentStorage bool
	StorageBackend           string
	MinIOEndpoint            string
	MinIOAccessKey           string
	MinIOSecretKey           string
	MinIOBucket              string
	MinIOBackupBucket        string
	MinIORegion              string
	MinIOUseSSL              bool
	MinIOAutoCreateBucket    bool
	JWTSecret                string
	AccessTokenTTL           time.Duration
	RefreshTokenTTL          time.Duration
	SubmitCooldown           time.Duration
	GameConcurrency          int
	MaxEquippedBadges        int
	RatingScanInterval       time.Duration
	RatingJudgeGrace         time.Duration
	AIEncryptionKey          string
	AIKeyFile                string
	AIRequestTimeout         time.Duration
	AICooldown               time.Duration
	MailEncryptionKey        string
	MailKeyFile              string
	MailTimeout              time.Duration
	TrustedProxies           []string
	DevAutoMigrate           bool

	BackupDir                  string
	BackupMaxUploadBytes       int64
	BackupMaxUncompressedBytes int64
	BackupMaxEntries           int
}

const minSecretLen = 32

// loadConfigFile reads the single dotenv config when KAPOJ_CONFIG_FILE is set.
// Explicit non-empty environment variables take precedence, so a developer can
// override service hostnames for a local go run without editing the file.
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

func databaseURLFromPostgres(host, port string) string {
	user := os.Getenv("POSTGRES_USER")
	database := os.Getenv("POSTGRES_DB")
	if user == "" || database == "" {
		return ""
	}
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "5432"
	}
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, os.Getenv("POSTGRES_PASSWORD")),
		Host:     net.JoinHostPort(host, port),
		Path:     "/" + database,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

// Load reads the unified dotenv file (when KAPOJ_CONFIG_FILE is set) and then
// resolves KAPOJ_* environment variables. Non-empty environment variables win
// over file values, which keeps local development overrides possible.
func Load() (*Config, error) {
	if err := loadConfigFile(); err != nil {
		return nil, err
	}
	v := viper.New()
	v.SetEnvPrefix("KAPOJ")
	v.AutomaticEnv()

	v.SetDefault("http_addr", ":8080")
	v.SetDefault("database_url", "")
	v.SetDefault("db_host", "localhost")
	v.SetDefault("db_port", "5432")
	v.SetDefault("redis_addr", "localhost:6379")
	v.SetDefault("storage_dir", "data")
	v.SetDefault("require_persistent_storage", false)
	v.SetDefault("storage_backend", "local")
	v.SetDefault("minio_endpoint", "localhost:9000")
	v.SetDefault("minio_access_key", "")
	v.SetDefault("minio_secret_key", "")
	v.SetDefault("minio_bucket", "kapoj")
	v.SetDefault("minio_backup_bucket", "kapoj-backups")
	v.SetDefault("minio_region", "")
	v.SetDefault("minio_use_ssl", false)
	v.SetDefault("minio_auto_create_bucket", true)
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
	v.SetDefault("dev_automigrate", false)
	v.SetDefault("backup_dir", "")
	v.SetDefault("backup_max_upload_bytes", int64(2<<30))
	v.SetDefault("backup_max_uncompressed_bytes", int64(20<<30))
	v.SetDefault("backup_max_entries", 200_000)

	trustedProxies, err := parseTrustedProxies(v.GetString("trusted_proxies"))
	if err != nil {
		return nil, err
	}
	databaseURL := v.GetString("database_url")
	if databaseURL == "" {
		databaseURL = databaseURLFromPostgres(v.GetString("db_host"), v.GetString("db_port"))
	}

	cfg := &Config{
		HTTPAddr:                   v.GetString("http_addr"),
		DatabaseURL:                databaseURL,
		RedisAddr:                  v.GetString("redis_addr"),
		StorageDir:                 v.GetString("storage_dir"),
		RequirePersistentStorage:   v.GetBool("require_persistent_storage"),
		StorageBackend:             v.GetString("storage_backend"),
		MinIOEndpoint:              v.GetString("minio_endpoint"),
		MinIOAccessKey:             v.GetString("minio_access_key"),
		MinIOSecretKey:             v.GetString("minio_secret_key"),
		MinIOBucket:                v.GetString("minio_bucket"),
		MinIOBackupBucket:          v.GetString("minio_backup_bucket"),
		MinIORegion:                v.GetString("minio_region"),
		MinIOUseSSL:                v.GetBool("minio_use_ssl"),
		MinIOAutoCreateBucket:      v.GetBool("minio_auto_create_bucket"),
		JWTSecret:                  v.GetString("jwt_secret"),
		AccessTokenTTL:             v.GetDuration("access_token_ttl"),
		RefreshTokenTTL:            v.GetDuration("refresh_token_ttl"),
		SubmitCooldown:             v.GetDuration("submit_cooldown"),
		GameConcurrency:            v.GetInt("game_concurrency"),
		MaxEquippedBadges:          v.GetInt("max_equipped_badges"),
		RatingScanInterval:         v.GetDuration("rating_scan_interval"),
		RatingJudgeGrace:           v.GetDuration("rating_judge_grace"),
		AIEncryptionKey:            v.GetString("ai_encryption_key"),
		AIKeyFile:                  v.GetString("ai_key_file"),
		AIRequestTimeout:           v.GetDuration("ai_request_timeout"),
		AICooldown:                 v.GetDuration("ai_cooldown"),
		MailEncryptionKey:          v.GetString("mail_encryption_key"),
		MailKeyFile:                v.GetString("mail_key_file"),
		MailTimeout:                v.GetDuration("mail_timeout"),
		TrustedProxies:             trustedProxies,
		DevAutoMigrate:             v.GetBool("dev_automigrate"),
		BackupDir:                  v.GetString("backup_dir"),
		BackupMaxUploadBytes:       v.GetInt64("backup_max_upload_bytes"),
		BackupMaxUncompressedBytes: v.GetInt64("backup_max_uncompressed_bytes"),
		BackupMaxEntries:           v.GetInt("backup_max_entries"),
	}

	if cfg.MinIOAccessKey == "" {
		cfg.MinIOAccessKey = os.Getenv("MINIO_ROOT_USER")
	}
	if cfg.MinIOSecretKey == "" {
		cfg.MinIOSecretKey = os.Getenv("MINIO_ROOT_PASSWORD")
	}
	if len(cfg.JWTSecret) < minSecretLen {
		return nil, errors.New("KAPOJ_JWT_SECRET must be at least 32 bytes")
	}
	switch cfg.StorageBackend {
	case "local":
	case "minio":
		if cfg.MinIOEndpoint == "" {
			return nil, errors.New("KAPOJ_MINIO_ENDPOINT is required when KAPOJ_STORAGE_BACKEND=minio")
		}
		if cfg.MinIOAccessKey == "" || cfg.MinIOSecretKey == "" {
			return nil, errors.New("KAPOJ_MINIO_ACCESS_KEY and KAPOJ_MINIO_SECRET_KEY are required when KAPOJ_STORAGE_BACKEND=minio")
		}
		if cfg.MinIOBucket == "" || cfg.MinIOBackupBucket == "" {
			return nil, errors.New("KAPOJ_MINIO_BUCKET and KAPOJ_MINIO_BACKUP_BUCKET are required when KAPOJ_STORAGE_BACKEND=minio")
		}
		if cfg.MinIOBucket == cfg.MinIOBackupBucket {
			return nil, errors.New("KAPOJ_MINIO_BUCKET and KAPOJ_MINIO_BACKUP_BUCKET must be different")
		}
	default:
		return nil, fmt.Errorf("KAPOJ_STORAGE_BACKEND must be local or minio, got %q", cfg.StorageBackend)
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
