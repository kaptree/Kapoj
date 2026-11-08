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
	full, err := resolve(root, key)
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

// resolve maps a storage key to an absolute path under root, rejecting any key
// that could escape the root. Shared by Open, Put and RemoveAll so that every
// storage operation validates keys identically.
func resolve(root, key string) (string, error) {
	if key == "" || strings.ContainsAny(key, "\x00\\") || strings.HasPrefix(key, "/") {
		return "", ErrInvalidKey
	}
	if path.Clean(key) != key || key == "." || strings.HasPrefix(key, "../") {
		return "", ErrInvalidKey
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	full := filepath.Join(abs, filepath.FromSlash(key))
	if !strings.HasPrefix(full, abs+string(filepath.Separator)) {
		return "", ErrInvalidKey
	}
	return full, nil
}
