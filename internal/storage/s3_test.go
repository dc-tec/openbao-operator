package storage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"gocloud.dev/blob/memblob"
)

func TestOpenS3Bucket_WithCredentials(t *testing.T) {
	ctx := context.Background()

	bucket, err := OpenS3Bucket(ctx, S3ClientConfig{
		Endpoint:        "https://s3.amazonaws.com",
		Bucket:          "test-bucket",
		Region:          "us-west-2",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		SessionToken:    "test-session-token",
	})
	if err != nil {
		t.Fatalf("OpenS3Bucket() error = %v", err)
	}
	defer func() {
		_ = bucket.Close()
	}()

	if bucket == nil {
		t.Fatal("OpenS3Bucket() should return bucket")
	}
}

func TestOpenS3Bucket_MissingRegion(t *testing.T) {
	ctx := context.Background()

	// Region is required, so this should error
	_, err := OpenS3Bucket(ctx, S3ClientConfig{
		Endpoint: "https://s3.amazonaws.com",
		Bucket:   "test-bucket",
		// Region is missing
	})
	if err == nil {
		t.Fatal("OpenS3Bucket() should error when region is missing")
	}

	expectedErr := "region is required"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("OpenS3Bucket() error = %v, want error containing %q", err, expectedErr)
	}
}

// ============================================================================
// Bucket Tests (using memblob for in-memory testing)
// ============================================================================

// TestBucket_Upload verifies that Upload correctly stores content.
func TestBucket_Upload(t *testing.T) {
	ctx := context.Background()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() {
		_ = bucket.Close()
	}()

	data := []byte("test backup data")
	reader := bytes.NewReader(data)

	if err := bucket.Upload(ctx, "test-key", reader); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	downloaded, err := bucket.Download(ctx, "test-key")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer func() {
		_ = downloaded.Close()
	}()

	content, err := io.ReadAll(downloaded)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if !bytes.Equal(content, data) {
		t.Errorf("Content mismatch: got %q, want %q", content, data)
	}
}

// TestBucket_Delete verifies that Delete removes objects.
func TestBucket_Delete(t *testing.T) {
	ctx := context.Background()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() {
		_ = bucket.Close()
	}()

	data := []byte("test data")
	if err := bucket.Upload(ctx, "delete-test", bytes.NewReader(data)); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	if err := bucket.Delete(ctx, "delete-test"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	info, err := bucket.Head(ctx, "delete-test")
	if err != nil {
		t.Fatalf("Head() after delete error = %v", err)
	}
	if info != nil {
		t.Error("Object should not exist after delete")
	}
}

// TestBucket_DeleteNonExistent verifies deleting non-existent objects doesn't error.
func TestBucket_DeleteNonExistent(t *testing.T) {
	ctx := context.Background()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() {
		_ = bucket.Close()
	}()

	if err := bucket.Delete(ctx, "non-existent-key"); err != nil {
		t.Errorf("Delete() of non-existent key should not error, got: %v", err)
	}
}

// TestBucket_List verifies that List returns objects with correct prefix.
func TestBucket_List(t *testing.T) {
	ctx := context.Background()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() {
		_ = bucket.Close()
	}()

	testData := []struct {
		key  string
		data []byte
	}{
		{"backups/cluster1/backup1", []byte("backup1")},
		{"backups/cluster1/backup2", []byte("backup2")},
		{"backups/cluster2/backup1", []byte("backup3")},
	}

	for _, td := range testData {
		if err := bucket.Upload(ctx, td.key, bytes.NewReader(td.data)); err != nil {
			t.Fatalf("Upload() for %s error = %v", td.key, err)
		}
	}

	objects, err := bucket.List(ctx, "backups/cluster1/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(objects) != 2 {
		t.Errorf("List() returned %d objects, want 2", len(objects))
	}
}

// TestBucket_Head verifies that Head returns correct metadata.
func TestBucket_Head(t *testing.T) {
	ctx := context.Background()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() {
		_ = bucket.Close()
	}()

	info, err := bucket.Head(ctx, "non-existent")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if info != nil {
		t.Error("Head() on non-existent object should return nil")
	}

	data := []byte("test data with known size")
	if err := bucket.Upload(ctx, "head-test", bytes.NewReader(data)); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	info, err = bucket.Head(ctx, "head-test")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if info == nil {
		t.Fatal("Head() should return metadata for existing object")
	}
	if info.Size != int64(len(data)) {
		t.Errorf("Head() Size = %d, want %d", info.Size, len(data))
	}
}
