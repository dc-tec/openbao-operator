package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"gocloud.dev/blob/memblob"
)

// failingReader returns data once, then a read error, like a snapshot stream cut mid-transfer.
type failingReader struct {
	data string
	err  error
	done bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.data), nil
}

func TestBucketUpload_ReadFailureDoesNotCommitObject(t *testing.T) {
	t.Parallel()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() { _ = bucket.Close() }()

	streamErr := errors.New("snapshot stream failed")
	err := bucket.Upload(context.Background(), "backups/partial.snap", &failingReader{data: "partial-snapshot", err: streamErr})
	if !errors.Is(err, streamErr) {
		t.Fatalf("Upload() error = %v, want %v", err, streamErr)
	}

	info, err := bucket.Head(context.Background(), "backups/partial.snap")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if info != nil {
		t.Fatalf("partial upload was committed with %d bytes", info.Size)
	}
}

func TestBucketUpload_CompleteStreamCommitsObject(t *testing.T) {
	t.Parallel()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() { _ = bucket.Close() }()

	if err := bucket.Upload(context.Background(), "backups/complete.snap", io.NopCloser(strings.NewReader("complete-snapshot"))); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	info, err := bucket.Head(context.Background(), "backups/complete.snap")
	if err != nil || info == nil {
		t.Fatalf("Head() = %v, %v; want committed object", info, err)
	}
	if info.Size != int64(len("complete-snapshot")) {
		t.Fatalf("object size = %d, want %d", info.Size, len("complete-snapshot"))
	}
}
