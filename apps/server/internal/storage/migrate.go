package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxMigrationObjectBytes = int64(1) << 40

// MigrateResult summarizes a local-to-object-store copy.
type MigrateResult struct {
	Copied  int64
	Skipped int64
}

// MigrateLocalToStore copies every business file from sourceDir into dst while
// preserving keys. Local secrets and backup archives are intentionally skipped.
// The source tree is never modified, so an operator can retry or roll back.
func MigrateLocalToStore(ctx context.Context, sourceDir string, dst ObjectStore) (MigrateResult, error) {
	result := MigrateResult{}
	abs, err := filepath.Abs(sourceDir)
	if err != nil {
		return result, err
	}
	err = filepath.WalkDir(abs, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".restore-") {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			result.Skipped++
			return nil
		}
		rel, err := filepath.Rel(abs, full)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !isBusinessObjectKey(key) {
			result.Skipped++
			return nil
		}
		file, err := os.Open(full)
		if err != nil {
			return fmt.Errorf("open %s: %w", key, err)
		}
		_, putErr := dst.Put(ctx, key, file, maxMigrationObjectBytes)
		closeErr := file.Close()
		if putErr != nil {
			return fmt.Errorf("copy %s: %w", key, putErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", key, closeErr)
		}
		result.Copied++
		return nil
	})
	return result, err
}

// Business objects live under a small, explicit set of prefixes. An allowlist
// is safer than a denylist: custom AI/mail key paths, rotation backups, and
// other local artifacts can never be copied into the object store by accident.
var businessObjectPrefixes = []string{
	"badges/",
	"avatars/",
	"certificates/",
	"site/",
	"problems/",
}

func isBusinessObjectKey(key string) bool {
	for _, prefix := range businessObjectPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
