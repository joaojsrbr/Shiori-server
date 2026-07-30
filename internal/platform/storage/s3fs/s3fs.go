package s3fs

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Provider struct {
	client *minio.Client
	bucket string
}

func New(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Provider, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing minio client: %w", err)
	}

	// Ensure bucket exists (simplified)
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err == nil && !exists {
		err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("creating bucket: %w", err)
		}
	}

	return &Provider{
		client: client,
		bucket: bucket,
	}, nil
}

func (p *Provider) Put(ctx context.Context, key string, r io.Reader) error {
	// Determine size dynamically or read into buffer. minio-go PutObject prefers known sizes.
	// For this adapter, we'll use -1 and let minio-go handle chunking (requires stream).
	_, err := p.client.PutObject(ctx, p.bucket, key, r, -1, minio.PutObjectOptions{
		ContentType: "application/octet-stream", // Should be determined in a real implementation
	})
	return err
}

func (p *Provider) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := p.client.GetObject(ctx, p.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	// MinIO client doesn't return an error immediately on GetObject if the key doesn't exist,
	// it errors on the first read. But we can stat it to be sure.
	_, err = obj.Stat()
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func (p *Provider) Delete(ctx context.Context, key string) error {
	return p.client.RemoveObject(ctx, p.bucket, key, minio.RemoveObjectOptions{})
}
