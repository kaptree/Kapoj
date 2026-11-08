package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// ErrTooLarge is returned by Put when the data exceeds maxBytes.
var ErrTooLarge = errors.New("storage: object exceeds maximum size")

// Put writes the contents of r to key under root. The write is atomic: data is
// staged in a temporary file and renamed into place only after it is known to
// fit within maxBytes, so a too-large write never clobbers an existing object.
// It returns the number of bytes written.
func Put(root, key string, r io.Reader, maxBytes int64) (int64, error) {
	full, err := resolve(root, key)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".put-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	n, err := io.Copy(tmp, io.LimitReader(r, maxBytes+1))
	if err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if n > maxBytes {
		_ = tmp.Close()
		return 0, ErrTooLarge
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmpName, full); err != nil {
		return 0, err
	}
	return n, nil
}

// RemoveAll removes key (a file or directory) under root. Unsafe keys are
// rejected exactly like Open, so removal can never escape the root.
func RemoveAll(root, key string) error {
	full, err := resolve(root, key)
	if err != nil {
		return err
	}
	return os.RemoveAll(full)
}
