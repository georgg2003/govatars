package s3

import (
	"context"
	"errors"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/config"
)

// Client wraps MinIO / S3 client for object operations and health checks.
type Client struct {
	inner  *minio.Client
	bucket string
	region string
}

// New builds an S3-compatible client and ensures the configured bucket exists.
func New(ctx context.Context, cfg config.S3) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("s3: empty endpoint")
	}
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, apperr.Wrap(err, "s3: new client")
	}
	c := &Client{inner: cli, bucket: cfg.Bucket, region: cfg.Region}
	if err := c.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) ensureBucket(ctx context.Context) error {
	if c.bucket == "" {
		return errors.New("s3: empty bucket name")
	}
	ok, err := c.inner.BucketExists(ctx, c.bucket)
	if err != nil {
		return apperr.Wrap(err, "s3: bucket exists check")
	}
	if ok {
		return nil
	}
	if err := c.inner.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{Region: c.region}); err != nil {
		return apperr.Wrap(err, "s3: make bucket")
	}
	return nil
}

// Health verifies connectivity (bucket must exist for a strict check).
func (c *Client) Health(ctx context.Context) error {
	if c.bucket == "" {
		_, err := c.inner.ListBuckets(ctx)
		return apperr.Wrap(err, "s3 list buckets")
	}
	ok, err := c.inner.BucketExists(ctx, c.bucket)
	if err != nil {
		return apperr.Wrap(err, "s3 bucket exists")
	}
	if !ok {
		return fmt.Errorf("bucket %q does not exist", c.bucket)
	}
	return nil
}
