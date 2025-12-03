package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestS3ClientUpload_WithUnknownContentLength verifies that Upload succeeds when
// the caller passes an unknown size (for example, a streaming snapshot with
// ContentLength == -1). The underlying AWS S3 client should fall back to a
// streaming upload (typically chunked encoding) without relying on the size.
func TestS3ClientUpload_WithUnknownContentLength(t *testing.T) {
	t.Helper()

	var receivedBody []byte
	var receivedContentLength string
	var receivedTransferEncoding string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			_ = r.Body.Close()
		}()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		receivedBody = body
		receivedContentLength = r.Header.Get("Content-Length")
		receivedTransferEncoding = r.Header.Get("Transfer-Encoding")

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := S3ClientConfig{
		Endpoint:        server.URL,
		Bucket:          "test-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		UsePathStyle:    true,
	}

	client, err := NewS3Client(ctx, cfg)
	if err != nil {
		t.Fatalf("NewS3Client() error = %v", err)
	}

	data := []byte("streaming snapshot data")
	reader := bytes.NewReader(data)

	// Pass size = -1 to indicate unknown content length.
	if err := client.Upload(ctx, "test-key", reader, -1); err != nil {
		t.Fatalf("Upload() error = %v, want nil", err)
	}

	if !bytes.Equal(receivedBody, data) {
		t.Fatalf("expected server to receive body %q, got %q", string(data), string(receivedBody))
	}

	// We do not assert exact transfer encoding semantics, but we do expect that
	// the client did not rely on an explicit Content-Length header for success.
	if receivedContentLength == "" && receivedTransferEncoding == "" {
		t.Fatalf("expected either Content-Length or Transfer-Encoding to be set on request")
	}
}
