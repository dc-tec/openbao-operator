// Package interfaces defines service interfaces for dependency injection.
// This package enables loose coupling between components and facilitates testing.
package interfaces

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

// BlobStore provides an interface for object storage operations.
// Implementations include S3, Azure Blob Storage, GCS, and in-memory stores for testing.
type BlobStore interface {
	// Upload stores the contents of body as an object with the given key.
	// For large objects, implementations may use multipart uploads.
	Upload(ctx context.Context, key string, body io.Reader) error

	// Download retrieves an object and returns a reader for its contents.
	// The caller is responsible for closing the returned ReadCloser.
	// Returns an error if the object does not exist.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object with the given key.
	// Returns nil if the object does not exist.
	Delete(ctx context.Context, key string) error

	// DeleteBatch removes multiple objects at once.
	// This is a convenience method for deleting multiple objects.
	DeleteBatch(ctx context.Context, keys []string) error

	// List returns metadata for all objects matching the given prefix.
	// Results are sorted by key name ascending.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// Head retrieves metadata for a single object without downloading its contents.
	// Returns nil and no error if the object does not exist.
	Head(ctx context.Context, key string) (*ObjectInfo, error)

	// Close releases any resources held by the store.
	Close() error
}
