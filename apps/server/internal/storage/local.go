package storage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
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
	if key == "" || strings.ContainsAny(key, "\x00\\") || strings.HasPrefix(key, "/") {
		return "", ErrInvalidKey
	}
	if path.Clean(key) != key || key == "." || strings.HasPrefix(key, "../") {
		return "", ErrInvalidKey
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
