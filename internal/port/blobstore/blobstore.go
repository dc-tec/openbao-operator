package blobstore

import (
	"context"
	"io"
	"time"
)

// ObjectInfo contains metadata about an object in storage.
type ObjectInfo struct {
	// Key is the full object key/path in the bucket.
	Key string
	// Size is the object size in bytes.
	Size int64
	// LastModified is when the object was last modified.
	LastModified time.Time
	// ETag is the entity tag for the object (typically an MD5 hash).
	ETag string
}

// BlobStore provides object storage operations.
type BlobStore interface {
	// Upload stores the contents of body as an object with the given key.
	Upload(ctx context.Context, key string, body io.Reader) error

	// Download retrieves an object and returns a reader for its contents.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object with the given key.
	Delete(ctx context.Context, key string) error

	// DeleteBatch removes multiple objects at once.
	DeleteBatch(ctx context.Context, keys []string) error

	// List returns metadata for all objects matching the given prefix.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// Head retrieves metadata for a single object without downloading its contents.
	Head(ctx context.Context, key string) (*ObjectInfo, error)

	// Close releases any resources held by the store.
	Close() error
}
