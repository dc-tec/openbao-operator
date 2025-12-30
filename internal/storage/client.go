// Package storage provides cloud-agnostic object storage interfaces and implementations
// for backup operations in the OpenBao Operator.
package storage

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

// ObjectStorage defines the interface for cloud-agnostic object storage operations.
// Implementations must be safe for concurrent use.
type ObjectStorage interface {
	// Upload stores the contents of body as an object with the given key.
	// The size parameter is the expected size of the content; pass -1 if unknown.
	// For large objects (>100MB), implementations should use multipart upload.
	Upload(ctx context.Context, key string, body io.Reader, size int64) error

	// Delete removes the object with the given key.
	// Returns nil if the object does not exist.
	Delete(ctx context.Context, key string) error

	// List returns metadata for all objects matching the given prefix.
	// Results are sorted by key name ascending.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// Head retrieves metadata for a single object without downloading its contents.
	// Returns nil and no error if the object does not exist.
	Head(ctx context.Context, key string) (*ObjectInfo, error)

	// Download retrieves an object and returns a reader for its contents.
	// The caller is responsible for closing the returned ReadCloser.
	// Returns an error if the object does not exist.
	Download(ctx context.Context, key string) (io.ReadCloser, error)
}
