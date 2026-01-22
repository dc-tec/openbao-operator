package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gocloud.dev/blob/memblob"
)

func TestOpenS3Bucket_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    S3ClientConfig
		wantError string
	}{
		{
			name: "Valid Configuration",
			config: S3ClientConfig{
				Endpoint:        "https://s3.amazonaws.com",
				Bucket:          "test-bucket",
				Region:          "us-west-2",
				AccessKeyID:     "test",
				SecretAccessKey: "test",
			},
			wantError: "",
		},
		{
			name: "Missing Bucket",
			config: S3ClientConfig{
				Endpoint: "https://s3.amazonaws.com",
				Region:   "us-west-2",
			},
			wantError: "bucket is required",
		},
		{
			name: "Missing Region",
			config: S3ClientConfig{
				Endpoint: "https://s3.amazonaws.com",
				Bucket:   "test-bucket",
			},
			wantError: "region is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			bucket, err := OpenS3Bucket(ctx, tt.config)

			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("OpenS3Bucket() error = nil, want error containing %q", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Errorf("OpenS3Bucket() error = %v, want error containing %q", err, tt.wantError)
				}
				return
			}

			if err != nil {
				t.Fatalf("OpenS3Bucket() unexpected error = %v", err)
			}
			if bucket == nil {
				t.Error("OpenS3Bucket() returned nil bucket")
			}
			_ = bucket.Close()
		})
	}
}

// ============================================================================
// Bucket Tests (using memblob for in-memory testing)
// ============================================================================

// TestBucket_Operations verifies the Bucket wrapper functionality using memblob.
func TestBucket_Operations(t *testing.T) {
	ctx := context.Background()
	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() { _ = bucket.Close() }()

	t.Run("Upload and Download", func(t *testing.T) {
		data := []byte("test backup data")
		key := "test-key"

		if err := bucket.Upload(ctx, key, bytes.NewReader(data)); err != nil {
			t.Fatalf("Upload() error = %v", err)
		}

		downloaded, err := bucket.Download(ctx, key)
		if err != nil {
			t.Fatalf("Download() error = %v", err)
		}
		defer func() { _ = downloaded.Close() }()

		content, err := io.ReadAll(downloaded)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}

		if !bytes.Equal(content, data) {
			t.Errorf("Content mismatch: got %q, want %q", content, data)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		key := "delete-test"
		data := []byte("test data")
		if err := bucket.Upload(ctx, key, bytes.NewReader(data)); err != nil {
			t.Fatalf("Upload() error = %v", err)
		}

		if err := bucket.Delete(ctx, key); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		info, err := bucket.Head(ctx, key)
		if err != nil {
			t.Fatalf("Head() after delete error = %v", err)
		}
		if info != nil {
			t.Error("Object should not exist after delete")
		}
	})

	t.Run("Delete Non-Existent", func(t *testing.T) {
		if err := bucket.Delete(ctx, "non-existent-key"); err != nil {
			t.Errorf("Delete() of non-existent key should not error, got: %v", err)
		}
	})

	t.Run("List", func(t *testing.T) {
		// Clear bucket first if shared (memblob is new each test in original, but here we share 'bucket' var?
		// Actually, I should probably new up a bucket per subtest to be clean or carefully manage keys.)
		// Since memblob is fast, let's just use separate buckets per subtest or distinct prefixes.

		// For List, let's use a specific prefix to avoid collision with other tests if we share bucket.
		prefix := "list-test/"
		testData := []struct {
			key  string
			data []byte
		}{
			{prefix + "backup1", []byte("backup1")},
			{prefix + "backup2", []byte("backup2")},
			{"other-prefix/backup3", []byte("backup3")},
		}

		for _, td := range testData {
			if err := bucket.Upload(ctx, td.key, bytes.NewReader(td.data)); err != nil {
				t.Fatalf("Upload() for %s error = %v", td.key, err)
			}
		}

		objects, err := bucket.List(ctx, prefix)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}

		if len(objects) != 2 {
			t.Errorf("List() returned %d objects, want 2", len(objects))
		}
	})

	t.Run("Head", func(t *testing.T) {
		// Test Non-Existent
		info, err := bucket.Head(ctx, "head-non-existent")
		if err != nil {
			t.Fatalf("Head() error = %v", err)
		}
		if info != nil {
			t.Error("Head() on non-existent object should return nil")
		}

		// Test Existing
		key := "head-test"
		data := []byte("test data with known size")
		if err := bucket.Upload(ctx, key, bytes.NewReader(data)); err != nil {
			t.Fatalf("Upload() error = %v", err)
		}

		info, err = bucket.Head(ctx, key)
		if err != nil {
			t.Fatalf("Head() error = %v", err)
		}
		if info == nil {
			t.Fatal("Head() should return metadata for existing object")
		}
		if info.Size != int64(len(data)) {
			t.Errorf("Head() Size = %d, want %d", info.Size, len(data))
		}
	})
}

func TestOpenS3Bucket_Features(t *testing.T) {
	// Mock S3 Server for EnsureExists and TLS tests
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect HEAD request for bucket check
		if r.Method == "HEAD" && strings.Contains(r.URL.Path, "test-bucket") {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	tests := []struct {
		name      string
		config    S3ClientConfig
		wantError string
	}{
		{
			name: "InsecureSkipVerify True (Self-Signed Cert)",
			config: S3ClientConfig{
				Endpoint:           server.URL,
				Bucket:             "test-bucket",
				Region:             "us-east-1",
				AccessKeyID:        "test",
				SecretAccessKey:    "test",
				InsecureSkipVerify: true,
				EnsureExists:       true,
				UsePathStyle:       true,
			},
			wantError: "",
		},
		{
			name: "InsecureSkipVerify False (Self-Signed Cert Failure)",
			config: S3ClientConfig{
				Endpoint:           server.URL,
				Bucket:             "test-bucket",
				Region:             "us-east-1",
				AccessKeyID:        "test",
				SecretAccessKey:    "test",
				InsecureSkipVerify: false,
				EnsureExists:       true,
				UsePathStyle:       true,
			},
			wantError: "certificate signed by unknown authority",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := OpenS3Bucket(ctx, tt.config)

			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("OpenS3Bucket() error = nil, want error containing %q", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					// Error message format might vary by platform/Go version, check for generic TLS cues if not exact
					if !strings.Contains(err.Error(), "x509") && !strings.Contains(err.Error(), "tls") {
						t.Errorf("OpenS3Bucket() error = %v, want error containing %q", err, tt.wantError)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("OpenS3Bucket() unexpected error = %v", err)
			}
		})
	}
}
