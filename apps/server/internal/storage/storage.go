// Package storage abstracts where uploaded files (test data, checkers) live.
package storage

import (
	"context"
	"errors"
	"io"
)

var (
	ErrNotFound   = errors.New("storage: object not found")
	ErrTooLarge   = errors.New("storage: object too large")
	ErrInvalidKey = errors.New("storage: invalid key")
)

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
