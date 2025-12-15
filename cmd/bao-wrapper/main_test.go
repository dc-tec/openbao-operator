package main

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"testing"
)

// Test_getFileHash tests the getFileHash function with various file scenarios.
// This is a unit test focusing on deterministic file hashing logic.
func Test_getFileHash(t *testing.T) {
	tests := []struct {
		name      string
		setupFile func(t *testing.T) string // returns file path
		wantErr   bool
		verify    func(t *testing.T, hash []byte, filePath string) // optional verification function
	}{
		{
			name: "valid file with content",
			setupFile: func(t *testing.T) string {
				tmpFile, err := os.CreateTemp("", "test-*.txt")
				if err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}
				content := []byte("test content")
				if _, err := tmpFile.Write(content); err != nil {
					t.Fatalf("failed to write to temp file: %v", err)
				}
				if err := tmpFile.Close(); err != nil {
					t.Fatalf("failed to close temp file: %v", err)
				}
				return tmpFile.Name()
			},
			wantErr: false,
			verify: func(t *testing.T, hash []byte, filePath string) {
				// Verify hash matches expected SHA256
				f, err := os.Open(filePath)
				if err != nil {
					t.Fatalf("failed to open file for verification: %v", err)
				}
				defer func() { _ = f.Close() }()

				h := sha256.New()
				if _, err := io.Copy(h, f); err != nil {
					t.Fatalf("failed to compute hash for verification: %v", err)
				}
				expected := h.Sum(nil)

				if !bytes.Equal(hash, expected) {
					t.Errorf("getFileHash() = %x, want %x", hash, expected)
				}

				// Hash should be 32 bytes (SHA256)
				if len(hash) != 32 {
					t.Errorf("getFileHash() returned hash of length %d, want 32", len(hash))
				}
			},
		},
		{
			name: "non-existent file",
			setupFile: func(t *testing.T) string {
				return "/nonexistent/file/path"
			},
			wantErr: true,
		},
		{
			name: "empty file",
			setupFile: func(t *testing.T) string {
				tmpFile, err := os.CreateTemp("", "test-empty-*.txt")
				if err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}
				if err := tmpFile.Close(); err != nil {
					t.Fatalf("failed to close temp file: %v", err)
				}
				return tmpFile.Name()
			},
			wantErr: false,
			verify: func(t *testing.T, hash []byte, filePath string) {
				// Empty file should have a known SHA256 hash
				expected := sha256.Sum256([]byte{})
				if !bytes.Equal(hash, expected[:]) {
					t.Errorf("getFileHash() for empty file = %x, want %x", hash, expected[:])
				}
			},
		},
		{
			name: "file with binary content",
			setupFile: func(t *testing.T) string {
				tmpFile, err := os.CreateTemp("", "test-binary-*.bin")
				if err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}
				content := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
				if _, err := tmpFile.Write(content); err != nil {
					t.Fatalf("failed to write to temp file: %v", err)
				}
				if err := tmpFile.Close(); err != nil {
					t.Fatalf("failed to close temp file: %v", err)
				}
				return tmpFile.Name()
			},
			wantErr: false,
			verify: func(t *testing.T, hash []byte, filePath string) {
				// Verify hash is correct
				f, err := os.Open(filePath)
				if err != nil {
					t.Fatalf("failed to open file for verification: %v", err)
				}
				defer func() { _ = f.Close() }()

				h := sha256.New()
				if _, err := io.Copy(h, f); err != nil {
					t.Fatalf("failed to compute hash for verification: %v", err)
				}
				expected := h.Sum(nil)

				if !bytes.Equal(hash, expected) {
					t.Errorf("getFileHash() = %x, want %x", hash, expected)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFile(t)
			defer func() {
				if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
					t.Logf("failed to cleanup temp file: %v", err)
				}
			}()

			got, err := getFileHash(filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("getFileHash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.verify != nil {
				tt.verify(t, got, filePath)
			}
		})
	}
}

// Test_getFileHash_Consistency verifies that hashing the same file multiple times
// produces consistent results.
func Test_getFileHash_Consistency(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-consistency-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	content := []byte("consistent content")
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	// Get hash multiple times - should be consistent
	hash1, err := getFileHash(tmpFile.Name())
	if err != nil {
		t.Fatalf("getFileHash() failed: %v", err)
	}

	hash2, err := getFileHash(tmpFile.Name())
	if err != nil {
		t.Fatalf("getFileHash() failed: %v", err)
	}

	if !bytes.Equal(hash1, hash2) {
		t.Errorf("getFileHash() returned inconsistent hashes: %x vs %x", hash1, hash2)
	}
}

// Test_getFileHash_DetectsChanges verifies that file changes are detected
// by comparing hash values before and after modification.
func Test_getFileHash_DetectsChanges(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-change-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	content1 := []byte("original content")
	if _, err := tmpFile.Write(content1); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	hash1, err := getFileHash(tmpFile.Name())
	if err != nil {
		t.Fatalf("getFileHash() failed: %v", err)
	}

	// Modify the file
	tmpFile, err = os.OpenFile(tmpFile.Name(), os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("failed to reopen temp file: %v", err)
	}
	content2 := []byte("modified content")
	if _, err := tmpFile.Write(content2); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	hash2, err := getFileHash(tmpFile.Name())
	if err != nil {
		t.Fatalf("getFileHash() failed: %v", err)
	}

	if bytes.Equal(hash1, hash2) {
		t.Errorf("getFileHash() did not detect file change: both hashes are %x", hash1)
	}
}

// Test_getFileHash_DifferentFilesProduceDifferentHashes verifies that
// different file contents produce different hash values.
func Test_getFileHash_DifferentFilesProduceDifferentHashes(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{
			name:    "content A",
			content: []byte("content A"),
		},
		{
			name:    "content B",
			content: []byte("content B"),
		},
		{
			name:    "similar but different",
			content: []byte("content A "), // trailing space
		},
	}

	hashes := make(map[string][]byte)
	for _, tt := range tests {
		tmpFile, err := os.CreateTemp("", "test-diff-*.txt")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer func() { _ = os.Remove(tmpFile.Name()) }()

		if _, err := tmpFile.Write(tt.content); err != nil {
			t.Fatalf("failed to write to temp file: %v", err)
		}
		if err := tmpFile.Close(); err != nil {
			t.Fatalf("failed to close temp file: %v", err)
		}

		hash, err := getFileHash(tmpFile.Name())
		if err != nil {
			t.Fatalf("getFileHash() failed for %s: %v", tt.name, err)
		}

		// Check for collisions with previous hashes
		for prevName, prevHash := range hashes {
			if bytes.Equal(hash, prevHash) {
				t.Errorf(
					"getFileHash() produced same hash for different contents: %s and %s both produce %x",
					prevName,
					tt.name,
					hash,
				)
			}
		}

		hashes[tt.name] = hash
	}
}
