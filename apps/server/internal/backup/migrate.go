package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const maxMigratedPackageBytes = int64(1) << 40

// MigrateLegacyPackages copies old local backup ZIPs into the package object
// store and rewrites backup_packages/backup_restore_jobs paths to object keys.
// It is best-effort and idempotent; missing local files are left untouched.
func MigrateLegacyPackages(
	ctx context.Context,
	db *gorm.DB,
	sourceBackupDir string,
	dst storage.ObjectStore,
) (copied, updated int, err error) {
	absSource, err := filepath.Abs(sourceBackupDir)
	if err != nil {
		return 0, 0, err
	}
	if _, statErr := os.Stat(absSource); os.IsNotExist(statErr) {
		return 0, 0, nil
	} else if statErr != nil {
		return 0, 0, statErr
	}

	relativeKeys := make([]string, 0)
	err = filepath.WalkDir(absSource, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(absSource, full)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !isLegacyPackageKey(key) {
			return nil
		}
		file, err := os.Open(full)
		if err != nil {
			return err
		}
		_, putErr := dst.Put(ctx, key, file, maxMigratedPackageBytes)
		closeErr := file.Close()
		if putErr != nil {
			return fmt.Errorf("copy legacy backup %s: %w", key, putErr)
		}
		if closeErr != nil {
			return closeErr
		}
		copied++
		relativeKeys = append(relativeKeys, key)
		return nil
	})
	if err != nil {
		return copied, 0, err
	}
	sort.Slice(relativeKeys, func(i, j int) bool {
		return len(relativeKeys[i]) > len(relativeKeys[j])
	})

	type legacyRow struct {
		ID    int64
		Value string
	}
	update := func(table, column string, rows []legacyRow) error {
		for _, row := range rows {
			if row.Value == "" {
				continue
			}
			key, ok := legacyKeyForPath(row.Value, relativeKeys)
			if !ok {
				continue
			}
			if err := db.WithContext(ctx).Table(table).Where("id = ?", row.ID).
				Update(column, key).Error; err != nil {
				return err
			}
			updated++
		}
		return nil
	}

	var packages []struct {
		ID       int64
		Filename string
	}
	if err := db.WithContext(ctx).Table("backup_packages").
		Select("id, filename").Scan(&packages).Error; err != nil {
		return copied, updated, err
	}
	rows := make([]legacyRow, 0, len(packages))
	for _, pkg := range packages {
		rows = append(rows, legacyRow{ID: pkg.ID, Value: pkg.Filename})
	}
	if err := update("backup_packages", "filename", rows); err != nil {
		return copied, updated, err
	}

	var uploads []struct {
		ID           int64
		UploadedFile string
	}
	if err := db.WithContext(ctx).Table("backup_restore_jobs").
		Select("id, uploaded_file").Scan(&uploads).Error; err != nil {
		return copied, updated, err
	}
	rows = rows[:0]
	for _, job := range uploads {
		rows = append(rows, legacyRow{ID: job.ID, Value: job.UploadedFile})
	}
	if err := update("backup_restore_jobs", "uploaded_file", rows); err != nil {
		return copied, updated, err
	}
	return copied, updated, nil
}

func isLegacyPackageKey(key string) bool {
	if !strings.HasSuffix(strings.ToLower(key), ".zip") {
		return false
	}
	for _, prefix := range []string{"platform/", "accounts/", "safety/", "uploads/"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func legacyKeyForPath(path string, keys []string) (string, bool) {
	normalized := strings.ReplaceAll(path, "\\", "/")
	for _, key := range keys {
		if normalized == key || strings.HasSuffix(normalized, "/"+key) {
			return key, true
		}
	}
	return "", false
}
