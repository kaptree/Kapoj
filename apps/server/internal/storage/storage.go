// Package storage abstracts where uploaded files (test data, checkers) live.
package storage

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"
)

var (
	ErrNotFound   = errors.New("storage: object not found")
	ErrTooLarge   = errors.New("storage: object too large")
	ErrInvalidKey = errors.New("storage: invalid key")
)

// ObjectInfo describes one stored object.
type ObjectInfo struct {
	Key  string
	Size int64
}

// Storage keeps named blobs. Keys are "/"-separated relative paths.
type Storage interface {
	// Put stores up to maxBytes from r under key and returns the stored size.
	// It fails with ErrTooLarge, leaving any previous content untouched, when
	// r holds more than maxBytes.
	Put(ctx context.Context, key string, r io.Reader, maxBytes int64) (int64, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes key; deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
}

// ObjectStore adds the enumeration and bulk-delete operations used by the
// backup/restore subsystem. Feature services only need Storage.
type ObjectStore interface {
	Storage
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
	DeletePrefix(ctx context.Context, prefix string) error
}

// ValidateKey enforces the key format used by every backend. Keys are relative,
// slash-separated paths and may never escape the storage root.
func ValidateKey(key string) error {
	if key == "" || strings.ContainsAny(key, "\x00\\") || strings.HasPrefix(key, "/") {
		return ErrInvalidKey
	}
	if path.Clean(key) != key || key == "." || strings.HasPrefix(key, "../") {
		return ErrInvalidKey
	}
	return nil
}
