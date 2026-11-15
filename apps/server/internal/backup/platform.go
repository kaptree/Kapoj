package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// restorePlatform replaces the database and business objects from a platform ZIP.
func (s *Service) restorePlatform(ctx context.Context, zipPath string, actorID int64) error {
	reader, err := OpenReader(zipPath, s.maxEntries, s.maxUncompressed)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	if reader.Manifest.Kind != KindPlatform {
		return httpx.ErrValidation("不是全平台备份包")
	}
	currentHash, err := s.database.SchemaHash(ctx)
	if err != nil {
		return err
	}
	if currentHash != reader.Manifest.SchemaHash {
		return ErrSchemaMismatch
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

	if err := s.createSafetyPackage(ctx, actorID); err != nil {
		return fmt.Errorf("恢复前安全备份失败: %w", err)
	}

	if err := copyStoreToDir(ctx, s.files, previousFiles); err != nil {
		return fmt.Errorf("备份当前文件失败: %w", err)
	}
	rollbackFiles := func(cause error) error {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		clearErr := clearBusinessStore(rollbackCtx, s.files)
		copyErr := copyDirToStore(rollbackCtx, s.files, previousFiles)
		if clearErr == nil && copyErr == nil {
			return cause
		}
		return errors.Join(cause, fmt.Errorf("文件回滚失败: clear=%v copy=%v", clearErr, copyErr))
	}
	if err := clearBusinessStore(ctx, s.files); err != nil {
		return rollbackFiles(fmt.Errorf("清空当前文件失败: %w", err))
	}
	if err := copyDirToStore(ctx, s.files, newFiles); err != nil {
		return rollbackFiles(fmt.Errorf("恢复文件失败: %w", err))
	}
	if err := s.database.Restore(ctx, reader, reader.Manifest); err != nil {
		return rollbackFiles(err)
	}
	return nil
}

// createSafetyPackage writes a pre-restore snapshot to the backup bucket and
// keeps only the newest safety object.
func (s *Service) createSafetyPackage(ctx context.Context, actorID int64) error {
	stamp := time.Now().UnixNano()
	key := fmt.Sprintf("safety/platform-%d.zip", stamp)
	localPath := filepath.Join(s.workDir, fmt.Sprintf("kapoj-safety-%d.zip", stamp))
	defer func() { _ = os.Remove(localPath) }()

	manifest, err := s.createPlatformFile(ctx, localPath, KindPlatform)
	if err != nil {
		return err
	}
	size, err := s.uploadPackage(ctx, key, localPath)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	row := &BackupPackage{
		Kind: KindPlatform, Filename: key, SizeBytes: size,
		Status: PackageReady, SchemaHash: manifest.SchemaHash,
		CreatedBy: actorID, CreatedAt: now, CompletedAt: &now,
	}
	if err := s.repo.CreatePackage(ctx, row); err != nil {
		return err
	}
	slog.Info("backup: safety snapshot created", "key", key, "size", size)
	old, err := s.packages.List(ctx, "safety/")
	if err != nil {
		return err
	}
	for _, object := range old {
		if object.Key != key {
			_ = s.packages.Delete(ctx, object.Key)
		}
	}
	_ = s.repo.DeleteSafetyPackagesExcept(ctx, row.ID)
	return nil
}
