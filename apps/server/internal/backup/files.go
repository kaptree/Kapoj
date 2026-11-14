package backup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func addStorageFile(archive *Writer, entry, storageRoot, key string) error {
	full, err := storagePath(storageRoot, key)
	if err != nil {
		return err
	}
	f, err := os.Open(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	w, err := archive.Create(entry)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

// addStorageTree includes every business file under root, excluding backups and
// temporary restore directories.
func addStorageTree(archive *Writer, root, backupDir string) error {
	backupAbs, _ := filepath.Abs(backupDir)
	return filepath.WalkDir(root, func(full string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		abs, _ := filepath.Abs(full)
		if abs == backupAbs || strings.HasPrefix(abs, backupAbs+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".restore-") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		entry := "files/" + filepath.ToSlash(rel)
		f, err := os.Open(full)
		if err != nil {
			return err
		}
		w, err := archive.Create(entry)
		if err != nil {
			_ = f.Close()
			return err
		}
		_, copyErr := io.Copy(w, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
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

// copyStorageTree copies business files, excluding the backup directory.
func copyStorageTree(src, dst, backupDir string) error {
	backupAbs, _ := filepath.Abs(backupDir)
	return filepath.WalkDir(src, func(full string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		abs, _ := filepath.Abs(full)
		if abs == backupAbs || strings.HasPrefix(abs, backupAbs+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, full)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(full)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		_ = in.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

// clearStorage removes business files while preserving the backup directory.
func clearStorage(root, backupDir string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	backupAbs, _ := filepath.Abs(backupDir)
	for _, entry := range entries {
		full := filepath.Join(root, entry.Name())
		abs, _ := filepath.Abs(full)
		if abs == backupAbs || strings.HasPrefix(backupAbs, abs+string(filepath.Separator)) {
			continue
		}
		if err := os.RemoveAll(full); err != nil {
			return err
		}
	}
	return nil
}
