package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/db"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

// openStorage selects the business and backup object stores. In MinIO mode the
// server keeps no persistent business files locally; only ephemeral temp files
// and the AI/mail key files use the host directory.
func openStorage(cfg *config.Config) (storage.Storage, storage.ObjectStore, storage.ObjectStore, error) {
	if cfg.StorageBackend == "minio" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		business, err := storage.NewMinIO(storage.MinIOOptions{
			Endpoint: cfg.MinIOEndpoint, AccessKey: cfg.MinIOAccessKey,
			SecretKey: cfg.MinIOSecretKey, Bucket: cfg.MinIOBucket,
			Region: cfg.MinIORegion, UseSSL: cfg.MinIOUseSSL,
			AutoCreateBucket: cfg.MinIOAutoCreateBucket,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		if err := ensureBucket(ctx, business); err != nil {
			return nil, nil, nil, err
		}
		backups, err := storage.NewMinIO(storage.MinIOOptions{
			Endpoint: cfg.MinIOEndpoint, AccessKey: cfg.MinIOAccessKey,
			SecretKey: cfg.MinIOSecretKey, Bucket: cfg.MinIOBackupBucket,
			Region: cfg.MinIORegion, UseSSL: cfg.MinIOUseSSL,
			AutoCreateBucket: cfg.MinIOAutoCreateBucket,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		if err := ensureBucket(ctx, backups); err != nil {
			return nil, nil, nil, err
		}
		slog.Info("storage backend ready", "backend", "minio",
			"bucket", cfg.MinIOBucket, "backup_bucket", cfg.MinIOBackupBucket,
		)
		return business, business, backups, nil
	}

	local, err := storage.NewLocal(cfg.StorageDir)
	if err != nil {
		return nil, nil, nil, err
	}
	if cfg.RequirePersistentStorage {
		persistent, err := storage.IsPersistentMount(cfg.StorageDir)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("verify persistent storage at %q: %w", cfg.StorageDir, err)
		}
		if !persistent {
			return nil, nil, nil, fmt.Errorf(
				"KAPOJ_STORAGE_DIR %q is on the container writable layer; mount a persistent volume or bind mount before starting",
				cfg.StorageDir,
			)
		}
		slog.Info("storage persistence verified", "dir", cfg.StorageDir)
	}
	backupDir := cfg.BackupDir
	if backupDir == "" {
		backupDir = filepath.Join(cfg.StorageDir, "backups")
	}
	packages, err := storage.NewLocal(backupDir)
	if err != nil {
		return nil, nil, nil, err
	}
	return local, local, packages, nil
}

func ensureBucket(ctx context.Context, store *storage.MinIO) error {
	var lastErr error
	for attempt := 0; attempt < 60; attempt++ {
		if lastErr = store.EnsureBucket(ctx); lastErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w (last error: %v)", ctx.Err(), lastErr)
		case <-time.After(time.Second):
		}
	}
	return lastErr
}

// runMigrateStorage copies the legacy local business tree into MinIO without
// deleting the source. It also migrates legacy backup ZIPs and rewrites their
// database paths, so Postgres is required; Redis is not.
func runMigrateStorage() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.StorageBackend != "minio" {
		return fmt.Errorf("set KAPOJ_STORAGE_BACKEND=minio before running migrate-storage")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dst, err := storage.NewMinIO(storage.MinIOOptions{
		Endpoint: cfg.MinIOEndpoint, AccessKey: cfg.MinIOAccessKey,
		SecretKey: cfg.MinIOSecretKey, Bucket: cfg.MinIOBucket,
		Region: cfg.MinIORegion, UseSSL: cfg.MinIOUseSSL,
		AutoCreateBucket: cfg.MinIOAutoCreateBucket,
	})
	if err != nil {
		return err
	}
	if err := ensureBucket(ctx, dst); err != nil {
		return err
	}
	source := cfg.StorageDir
	if source == "" {
		source = "data"
	}
	result, err := storage.MigrateLocalToStore(context.Background(), source, dst)
	if err != nil {
		return err
	}

	if cfg.DatabaseURL == "" {
		return fmt.Errorf("KAPOJ_DATABASE_URL is required to migrate legacy backup records")
	}
	backups, err := storage.NewMinIO(storage.MinIOOptions{
		Endpoint: cfg.MinIOEndpoint, AccessKey: cfg.MinIOAccessKey,
		SecretKey: cfg.MinIOSecretKey, Bucket: cfg.MinIOBackupBucket,
		Region: cfg.MinIORegion, UseSSL: cfg.MinIOUseSSL,
		AutoCreateBucket: cfg.MinIOAutoCreateBucket,
	})
	if err != nil {
		return err
	}
	if err := ensureBucket(ctx, backups); err != nil {
		return err
	}
	backupSource := cfg.BackupDir
	if backupSource == "" {
		backupSource = filepath.Join(source, "backups")
	} else if _, err := os.Stat(backupSource); err != nil {
		return fmt.Errorf("legacy backup directory %q is not readable: %w", backupSource, err)
	}

	g, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database for legacy backup migration: %w", err)
	}
	if sqlDB, dbErr := g.DB(); dbErr == nil {
		defer func() { _ = sqlDB.Close() }()
	}
	legacyCopied, legacyUpdated, err := backup.MigrateLegacyPackages(
		context.Background(), g, backupSource, backups,
	)
	if err != nil {
		return fmt.Errorf("migrate legacy backups: %w", err)
	}

	slog.Info("storage migration complete", "source", source,
		"bucket", cfg.MinIOBucket,
		"copied", result.Copied, "skipped", result.Skipped,
		"legacy_backups_copied", legacyCopied,
		"legacy_backups_updated", legacyUpdated,
	)
	return nil
}
