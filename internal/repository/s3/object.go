package s3

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/minio/minio-go/v7"

	"govatars/internal/pkg/apperr"
	"govatars/internal/usecase"
)

const noKeyErrorCode = "NoSuchKey"

// PutObject uploads bytes to the configured bucket.
func (c *Client) PutObject(ctx context.Context, objectKey string, r io.Reader, size int64, contentType string) error {
	_, err := c.inner.PutObject(ctx, c.bucket, objectKey, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return apperr.Wrapf(err, "s3 put %q", objectKey)
	}
	return nil
}

// GetObject opens an object for reading. Caller must close the returned reader.
// Returns [usecase.ErrObjectNotFound] when the key is missing.
func (c *Client) GetObject(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	o, err := c.inner.GetObject(ctx, c.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, mapNotFound(apperr.Wrapf(err, "s3 get %q", objectKey), err)
	}
	return o, nil
}

// StatObject returns object size and ETag (quotes stripped) without reading the body.
// Returns [usecase.ErrObjectNotFound] when the key is missing.
func (c *Client) StatObject(ctx context.Context, objectKey string) (int64, string, error) {
	info, err := c.inner.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return 0, "", mapNotFound(apperr.Wrapf(err, "s3 stat %q", objectKey), err)
	}
	etag := strings.Trim(info.ETag, `"`)
	return info.Size, etag, nil
}

// RemoveObject deletes a single object.
func (c *Client) RemoveObject(ctx context.Context, objectKey string) error {
	err := c.inner.RemoveObject(ctx, c.bucket, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return apperr.Wrapf(err, "s3 remove %q", objectKey)
	}
	return nil
}

// mapNotFound joins [usecase.ErrObjectNotFound] when the underlying minio error is a 404 / NoSuchKey,
// so callers can [errors.Is] the sentinel without depending on minio types.
func mapNotFound(wrapped, raw error) error {
	resp := minio.ToErrorResponse(raw)
	if resp.StatusCode == http.StatusNotFound || resp.Code == noKeyErrorCode {
		return errors.Join(usecase.ErrObjectNotFound, wrapped)
	}
	return wrapped
}

var _ usecase.ObjectStorage = (*Client)(nil)
