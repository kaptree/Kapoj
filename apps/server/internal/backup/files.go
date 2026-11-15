package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

// storagePath validates a storage key and joins it to root.
func storagePath(root, key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || strings.ContainsAny(key, "\\\x00") {
		return "", errors.New("invalid storage key")
	}
	clean := filepath.ToSlash(filepath.Clean(key))
	if clean != key || clean == "." || strings.HasPrefix(clean, "../") {
		return "", errors.New("invalid storage key")
	}
	full := filepath.Join(root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid storage key")
	}
	return full, nil
}

// addStoreFile adds one object to the archive under entry. Missing objects are
// skipped, matching the old local implementation and leaving the reference to
// the startup storage audit.
func addStoreFile(ctx context.Context, archive *Writer, store storage.Storage, entry, key string) error {
	reader, err := store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	writer, err := archive.Create(entry)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, reader)
	return err
}

// addStoreObjects includes every business object, excluding protected local
// keys and old backup archives. The list is sorted for deterministic ZIPs.
func addStoreObjects(ctx context.Context, archive *Writer, store storage.ObjectStore) error {
	objects, err := store.List(ctx, "")
	if err != nil {
		return err
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	for _, object := range objects {
		if !isBusinessKey(object.Key) {
			continue
		}
		if err := addStoreFile(ctx, archive, store, "files/"+object.Key, object.Key); err != nil {
			return err
		}
	}
	return nil
}

// isBusinessKey is an allowlist for platform snapshots and restores. Custom
// key paths, rotation backups, and old local backup directories can never be
// written into the business bucket through a restore.
var businessKeyPrefixes = []string{
	"badges/",
	"avatars/",
	"certificates/",
	"site/",
	"problems/",
}

func isBusinessKey(key string) bool {
	for _, prefix := range businessKeyPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// extractStorage writes archive entries under prefix into dest. It never
// follows symlinks and refuses paths outside dest.
func extractStorage(reader *Reader, prefix, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for name := range reader.files {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(name, prefix)
		full, err := storagePath(dest, rel)
		if err != nil {
			return fmt.Errorf("unsafe backup path %q: %w", name, err)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		rc, err := reader.Open(name)
		if err != nil {
			return err
		}
		// O_EXCL prevents an attacker-controlled duplicate entry from
		// overwriting an earlier extracted file.
		out, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, os.ErrExist) {
			_ = rc.Close()
			return fmt.Errorf("duplicate backup entry %q", name)
		}
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		_ = rc.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

const maxRestoreObjectBytes = int64(1) << 40

// copyStoreToDir materializes business objects under dst for the short
// rollback window of a platform restore.
func copyStoreToDir(ctx context.Context, store storage.ObjectStore, dst string) error {
	objects, err := store.List(ctx, "")
	if err != nil {
		return err
	}
	for _, object := range objects {
		if !isBusinessKey(object.Key) {
			continue
		}
		reader, err := store.Open(ctx, object.Key)
		if err != nil {
			return err
		}
		target, err := storagePath(dst, object.Key)
		if err != nil {
			_ = reader.Close()
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			_ = reader.Close()
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			_ = reader.Close()
			return err
		}
		_, copyErr := io.Copy(out, reader)
		_ = reader.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// copyDirToStore uploads all regular files under src, preserving relative keys.
func copyDirToStore(ctx context.Context, store storage.ObjectStore, src string) error {
	return filepath.WalkDir(src, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(src, full)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !isBusinessKey(key) {
			return nil
		}
		file, err := os.Open(full)
		if err != nil {
			return err
		}
		_, putErr := store.Put(ctx, key, file, maxRestoreObjectBytes)
		closeErr := file.Close()
		if putErr != nil {
			return putErr
		}
		return closeErr
	})
}

// listDirKeys returns every regular file under src as a slash-separated key.
func listDirKeys(src string) ([]string, error) {
	keys := make([]string, 0)
	err := filepath.WalkDir(src, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(src, full)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if isBusinessKey(key) {
			keys = append(keys, key)
		}
		return nil
	})
	sort.Strings(keys)
	return keys, err
}

// snapshotStoreKeys copies the current version of each key to dst and reports
// which keys existed. It is used for the account restore rollback window.
func snapshotStoreKeys(
	ctx context.Context,
	store storage.Storage,
	keys []string,
	dst string,
) (map[string]bool, error) {
	existed := make(map[string]bool, len(keys))
	for _, key := range keys {
		reader, err := store.Open(ctx, key)
		if errors.Is(err, storage.ErrNotFound) {
			existed[key] = false
			continue
		}
		if err != nil {
			return nil, err
		}
		target, err := storagePath(dst, key)
		if err != nil {
			_ = reader.Close()
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			_ = reader.Close()
			return nil, err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			_ = reader.Close()
			return nil, err
		}
		_, copyErr := io.Copy(out, reader)
		_ = reader.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		existed[key] = true
	}
	return existed, nil
}

// restoreStoreKeys restores a snapshot, deleting keys that did not exist
// before the failed restore.
func restoreStoreKeys(
	ctx context.Context,
	store storage.ObjectStore,
	keys []string,
	previousDir string,
	existed map[string]bool,
) error {
	for _, key := range keys {
		if !existed[key] {
			if err := store.Delete(ctx, key); err != nil {
				return err
			}
			continue
		}
		full, err := storagePath(previousDir, key)
		if err != nil {
			return err
		}
		file, err := os.Open(full)
		if err != nil {
			return err
		}
		_, putErr := store.Put(ctx, key, file, maxRestoreObjectBytes)
		closeErr := file.Close()
		if putErr != nil {
			return putErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// clearBusinessStore removes business objects but never local secret files.
func clearBusinessStore(ctx context.Context, store storage.ObjectStore) error {
	objects, err := store.List(ctx, "")
	if err != nil {
		return err
	}
	for _, object := range objects {
		if !isBusinessKey(object.Key) {
			continue
		}
		if err := store.Delete(ctx, object.Key); err != nil {
			return err
		}
	}
	return nil
}
