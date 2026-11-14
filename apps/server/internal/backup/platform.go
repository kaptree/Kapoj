package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// restorePlatform replaces the database and business files from a platform ZIP.
func (s *Service) restorePlatform(ctx context.Context, zipPath string, actorID int64) error {
	reader, err := OpenReader(zipPath, s.maxEntries, s.maxUncompressed)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	if reader.Manifest.Kind != KindPlatform {
		return httpx.ErrValidation("不是全平台备份包")
	}

	temp, err := os.MkdirTemp("", "kapoj-restore-platform-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temp) }()
	newFiles := filepath.Join(temp, "files")
	previousFiles := filepath.Join(temp, "previous")
	if err := extractStorage(reader, "files/", newFiles); err != nil {
		return err
	}
	if err := os.MkdirAll(previousFiles, 0o755); err != nil {
		return err
	}

	// Safety backup before any destructive change.
	safetyPath := filepath.Join(s.backupDir, "safety", fmt.Sprintf("platform-%d.zip", time.Now().UnixNano()))
	safetyManifest, err := s.createPlatformFile(ctx, safetyPath, KindPlatform)
	if err != nil {
		return fmt.Errorf("恢复前安全备份失败: %w", err)
	}
	now := time.Now().UTC()
	safety := &BackupPackage{
		Kind: KindPlatform, Filename: safetyPath, SizeBytes: fileSize(safetyPath),
		Status: PackageReady, SchemaHash: safetyManifest.SchemaHash,
		CreatedBy: actorID, CreatedAt: now, CompletedAt: &now,
	}
	if err := s.repo.CreatePackage(ctx, safety); err != nil {
		return err
	}
	// Keep only the latest safety ZIP.
	oldSafety, _ := filepath.Glob(filepath.Join(s.backupDir, "safety", "platform-*.zip"))
	for _, name := range oldSafety {
		if name != safetyPath {
			_ = os.Remove(name)
		}
	}

	if err := copyStorageTree(s.storageDir, previousFiles, s.backupDir); err != nil {
		return fmt.Errorf("备份当前文件失败: %w", err)
	}
	rollbackFiles := func() {
		_ = clearStorage(s.storageDir, s.backupDir)
		_ = copyStorageTree(previousFiles, s.storageDir, s.backupDir)
	}
	if err := clearStorage(s.storageDir, s.backupDir); err != nil {
		return err
	}
	if err := copyStorageTree(newFiles, s.storageDir, s.backupDir); err != nil {
		rollbackFiles()
		return fmt.Errorf("恢复文件失败: %w", err)
	}
	if err := s.database.Restore(ctx, reader, reader.Manifest); err != nil {
		rollbackFiles()
		return err
	}
	return nil
}
