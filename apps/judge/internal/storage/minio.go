package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOOptions struct {
	Endpoint         string
	AccessKey        string
	SecretKey        string
	Bucket           string
	Region           string
	UseSSL           bool
	AutoCreateBucket bool
}

// MinIO is the judge's object-storage backend. It shares the server's bucket
// and key conventions.
type MinIO struct {
	client     *minio.Client
	bucket     string
	region     string
	autoCreate bool
}

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
	return &MinIO{
		client: client, bucket: opts.Bucket, region: opts.Region,
		autoCreate: opts.AutoCreateBucket,
	}, nil
}

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

func (m *MinIO) Put(ctx context.Context, key string, r io.Reader, maxBytes int64) (int64, error) {
	if err := ValidateKey(key); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp("", "kapoj-judge-minio-*")
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

func (m *MinIO) RemovePrefix(ctx context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	prefix := strings.TrimSuffix(key, "/")
	for object := range m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
		Prefix: prefix, Recursive: true,
	}) {
		if object.Err != nil {
			return object.Err
		}
		if strings.HasSuffix(object.Key, "/") {
			continue
		}
		// S3 listing is lexical; keep the boundary so generated/1 does not
		// delete generated/10.
		if !keyWithinPrefix(object.Key, prefix) {
			continue
		}
		if err := m.client.RemoveObject(ctx, m.bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// deleteBucket is used by integration tests to avoid leaking temporary buckets.
func (m *MinIO) deleteBucket(ctx context.Context) error {
	for object := range m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{Recursive: true}) {
		if object.Err != nil {
			return object.Err
		}
		if err := m.client.RemoveObject(ctx, m.bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
	}
	return m.client.RemoveBucket(ctx, m.bucket)
}

func keyWithinPrefix(key, prefix string) bool {
	cleaned := strings.TrimSuffix(prefix, "/")
	if cleaned == "" {
		return true
	}
	return key == cleaned || strings.HasPrefix(key, cleaned+"/")
}

func minioNotFound(err error) bool {
	response := minio.ToErrorResponse(err)
	switch response.Code {
	case "NoSuchKey", "NoSuchObject", "NoSuchBucket", "NotFound":
		return true
	}
	return errors.Is(err, os.ErrNotExist)
}
