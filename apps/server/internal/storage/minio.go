package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOOptions configures the object-storage backend.
type MinIOOptions struct {
	Endpoint         string
	AccessKey        string
	SecretKey        string
	Bucket           string
	Region           string
	UseSSL           bool
	AutoCreateBucket bool
}

// MinIO stores blobs in a private MinIO/S3 bucket. Keys keep the same
// slash-separated format as the local backend.
type MinIO struct {
	client     *minio.Client
	bucket     string
	region     string
	autoCreate bool
}

// NewMinIO builds a client. Network and bucket checks happen in EnsureBucket.
func NewMinIO(opts MinIOOptions) (*MinIO, error) {
	if opts.Endpoint == "" {
		return nil, errors.New("storage: MinIO endpoint is required")
	}
	if opts.AccessKey == "" || opts.SecretKey == "" {
		return nil, errors.New("storage: MinIO access key and secret key are required")
	}
	if opts.Bucket == "" {
		return nil, errors.New("storage: MinIO bucket is required")
	}
	client, err := minio.New(opts.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(opts.AccessKey, opts.SecretKey, ""),
		Secure:       opts.UseSSL,
		Region:       opts.Region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: create MinIO client: %w", err)
	}
	store := &MinIO{client: client, bucket: opts.Bucket}
	if opts.AutoCreateBucket {
		// EnsureBucket is called later with a retryable context by main.
		store.autoCreate = true
	}
	store.region = opts.Region
	return store, nil
}

// EnsureBucket creates the bucket when AutoCreateBucket is enabled.
func (m *MinIO) EnsureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("storage: check MinIO bucket %q: %w", m.bucket, err)
	}
	if exists {
		return nil
	}
	if !m.autoCreate {
		return fmt.Errorf("storage: MinIO bucket %q does not exist", m.bucket)
	}
	region := m.region
	if region == "" {
		region = "us-east-1"
	}
	if err := m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{Region: region}); err != nil {
		response := minio.ToErrorResponse(err)
		if response.Code == "BucketAlreadyOwnedByYou" || response.Code == "BucketAlreadyExists" {
			return nil
		}
		return fmt.Errorf("storage: create MinIO bucket %q: %w", m.bucket, err)
	}
	return nil
}

// Put mirrors the local backend's maxBytes semantics. Files are spooled to a
// local temporary file first so a too-large upload never reaches the bucket.
func (m *MinIO) Put(ctx context.Context, key string, r io.Reader, maxBytes int64) (int64, error) {
	if err := ValidateKey(key); err != nil {
		return 0, err
	}
	if file, ok := r.(*os.File); ok {
		return m.putFile(ctx, key, file, maxBytes)
	}
	tmp, err := os.CreateTemp("", "kapoj-minio-put-*")
	if err != nil {
		return 0, err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
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
	file, err := os.Open(name)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	if _, err := m.client.PutObject(ctx, m.bucket, key, file, n, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	}); err != nil {
		return 0, err
	}
	return n, nil
}

func (m *MinIO) putFile(ctx context.Context, key string, file *os.File, maxBytes int64) (int64, error) {
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	size := info.Size() - offset
	if size < 0 {
		return 0, ErrInvalidKey
	}
	if size > maxBytes {
		return 0, ErrTooLarge
	}
	if _, err := m.client.PutObject(ctx, m.bucket, key, file, size, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	}); err != nil {
		return 0, err
	}
	return size, nil
}

func (m *MinIO) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	object, err := m.client.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		if minioNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if _, err := object.Stat(); err != nil {
		_ = object.Close()
		if minioNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return object, nil
}

func (m *MinIO) Delete(ctx context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	return m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{})
}

func (m *MinIO) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	objects := make([]ObjectInfo, 0)
	for object := range m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
		Prefix: prefix, Recursive: true,
	}) {
		if object.Err != nil {
			return nil, object.Err
		}
		if strings.HasSuffix(object.Key, "/") {
			continue
		}
		objects = append(objects, ObjectInfo{Key: object.Key, Size: object.Size})
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

func (m *MinIO) DeletePrefix(ctx context.Context, prefix string) error {
	objects, err := m.List(ctx, prefix)
	if err != nil {
		return err
	}
	for _, object := range objects {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !keyWithinPrefix(object.Key, prefix) {
			continue
		}
		if err := m.Delete(ctx, object.Key); err != nil {
			return err
		}
	}
	return nil
}

// keyWithinPrefix applies path-segment boundaries. S3 prefix listing is
// lexical, so deleting "generated/1" must not touch "generated/10".
func keyWithinPrefix(key, prefix string) bool {
	cleaned := strings.TrimSuffix(prefix, "/")
	if cleaned == "" {
		return true
	}
	return key == cleaned || strings.HasPrefix(key, cleaned+"/")
}

// deleteBucket is used by integration tests to avoid leaking temporary buckets.
func (m *MinIO) deleteBucket(ctx context.Context) error {
	if err := m.DeletePrefix(ctx, ""); err != nil {
		return err
	}
	return m.client.RemoveBucket(ctx, m.bucket)
}

func minioNotFound(err error) bool {
	response := minio.ToErrorResponse(err)
	switch response.Code {
	case "NoSuchKey", "NoSuchObject", "NoSuchBucket", "NotFound":
		return true
	}
	return errors.Is(err, os.ErrNotExist)
}
