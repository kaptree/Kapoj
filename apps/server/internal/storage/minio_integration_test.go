package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinIOStoreIntegration(t *testing.T) {
	endpoint := os.Getenv("KAPOJ_TEST_MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("KAPOJ_TEST_MINIO_ENDPOINT not set; skipping MinIO integration test")
	}
	accessKey := os.Getenv("KAPOJ_TEST_MINIO_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "kapoj"
	}
	secretKey := os.Getenv("KAPOJ_TEST_MINIO_SECRET_KEY")
	if secretKey == "" {
		secretKey = "kapoj-minio-dev"
	}
	bucket := fmt.Sprintf("kapoj-test-%d", time.Now().UnixNano())
	store, err := NewMinIO(MinIOOptions{
		Endpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey,
		Bucket: bucket, AutoCreateBucket: true,
	})
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, store.EnsureBucket(ctx))
	t.Cleanup(func() { _ = store.deleteBucket(context.Background()) })

	_, err = store.Put(ctx, "badges/1", strings.NewReader("image-bytes"), 100)
	require.NoError(t, err)
	reader, err := store.Open(ctx, "badges/1")
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	_ = reader.Close()
	assert.Equal(t, "image-bytes", string(data))

	objects, err := store.List(ctx, "badges/")
	require.NoError(t, err)
	require.Len(t, objects, 1)
	assert.Equal(t, "badges/1", objects[0].Key)

	_, err = store.Put(ctx, "badges/2", strings.NewReader("too-large"), 4)
	require.ErrorIs(t, err, ErrTooLarge)
	_, err = store.Open(ctx, "badges/missing")
	require.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, store.DeletePrefix(ctx, "badges/"))
	objects, err = store.List(ctx, "badges/")
	require.NoError(t, err)
	assert.Empty(t, objects)

	_, err = store.Put(ctx, "problems/1/generated/1/a.in", strings.NewReader("one"), 100)
	require.NoError(t, err)
	_, err = store.Put(ctx, "problems/1/generated/10/a.in", strings.NewReader("ten"), 100)
	require.NoError(t, err)
	require.NoError(t, store.DeletePrefix(ctx, "problems/1/generated/1"))
	_, err = store.Open(ctx, "problems/1/generated/1/a.in")
	require.ErrorIs(t, err, ErrNotFound)
	remain, err := store.Open(ctx, "problems/1/generated/10/a.in")
	require.NoError(t, err)
	_ = remain.Close()
}
