package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// Local stores objects under a local root. It exists for tests and offline
// development; production uses MinIO.
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

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	return Open(l.root, key)
}

func (l *Local) Put(_ context.Context, key string, r io.Reader, maxBytes int64) (int64, error) {
	return Put(l.root, key, r, maxBytes)
}

func (l *Local) RemovePrefix(_ context.Context, key string) error {
	return RemoveAll(l.root, key)
}
