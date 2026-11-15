package storage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Local stores blobs as files under a root directory.
type Local struct{ root string }

func NewLocal(root string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &Local{root: abs}, nil
}

// resolve validates key and returns its absolute path inside the root.
func (l *Local) resolve(key string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	full := filepath.Join(l.root, filepath.FromSlash(key))
	if !strings.HasPrefix(full, l.root+string(filepath.Separator)) {
		return "", ErrInvalidKey
	}
	return full, nil
}

func (l *Local) Put(ctx context.Context, key string, r io.Reader, maxBytes int64) (int64, error) {
	full, err := l.resolve(key)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	n, err := io.Copy(tmp, io.LimitReader(r, maxBytes+1))
	if err != nil {
		return 0, err
	}
	if n > maxBytes {
		return 0, ErrTooLarge
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmpName, full); err != nil {
		return 0, err
	}
	committed = true
	return n, nil
}

func (l *Local) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	full, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (l *Local) Delete(ctx context.Context, key string) error {
	full, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// List walks the local tree and returns regular files whose keys begin with
// prefix. Prefix is a filter only; it is never joined to the root.
func (l *Local) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	objects := make([]ObjectInfo, 0)
	err := filepath.WalkDir(l.root, func(full string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(l.root, full)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		objects = append(objects, ObjectInfo{Key: key, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

// DeletePrefix removes a subtree. Keys are validated by resolve.
func (l *Local) DeletePrefix(ctx context.Context, prefix string) error {
	if prefix == "" {
		entries, err := os.ReadDir(l.root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(l.root, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	full, err := l.resolve(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
