// Package storage reads testcase files from the shared storage directory.
package storage

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidKey = errors.New("storage: invalid key")
	ErrNotFound   = errors.New("storage: object not found")
)

// Open opens key under root. Keys are validated exactly like the Web
// service's local storage so a corrupted database cannot escape the root.
func Open(root, key string) (io.ReadCloser, error) {
	if key == "" || strings.ContainsAny(key, "\x00\\") || strings.HasPrefix(key, "/") {
		return nil, ErrInvalidKey
	}
	if path.Clean(key) != key || key == "." || strings.HasPrefix(key, "../") {
		return nil, ErrInvalidKey
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(abs, filepath.FromSlash(key))
	if !strings.HasPrefix(full, abs+string(filepath.Separator)) {
		return nil, ErrInvalidKey
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
