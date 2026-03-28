package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// watchFileForChanges watches a file for changes and calls onChange when a change is detected.
// It respects context cancellation for clean shutdown.
func watchFileForChanges(ctx context.Context, path string, interval time.Duration, onChange func()) {
	lastHash, _ := getFileHash(path)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentHash, err := getFileHash(path)
			if err != nil {
				log.Printf("Error reading watch file: %v", err)
				continue
			}

			if len(lastHash) > 0 && !bytes.Equal(lastHash, currentHash) {
				onChange()
				lastHash = currentHash
			} else if len(lastHash) == 0 {
				lastHash = currentHash
			}
		}
	}
}

func getFileHash(path string) ([]byte, error) {
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("path %q contains path traversal", path)
	}
	f, err := os.Open(cleanPath) // #nosec G304 -- Path is validated and cleaned to prevent traversal
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}
