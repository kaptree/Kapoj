package storage

import (
	"context"
	"io"
)

// Store is the judge's object storage contract. The local implementation is
// used by unit tests and offline development; production uses MinIO.
type Store interface {
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Put(ctx context.Context, key string, r io.Reader, maxBytes int64) (int64, error)
	RemovePrefix(ctx context.Context, key string) error
}
