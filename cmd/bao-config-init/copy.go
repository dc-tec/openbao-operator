package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// copyWrapper copies the wrapper binary from source to /utils/bao-wrapper
// and sets executable permissions. This eliminates the need for shell commands
// in the init container, allowing it to use a distroless/static image (no shell).
func copyWrapper(sourcePath string) error {
	return copyBinary(sourcePath, pathWrapperBinary)
}

func copyProbe(sourcePath string) error {
	return copyBinary(sourcePath, pathProbeBinary)
}

func copyBinary(sourcePath, destPath string) error {
	const fileMode = 0o755

	cleanSourcePath := filepath.Clean(sourcePath)
	if strings.Contains(cleanSourcePath, "..") {
		return fmt.Errorf("source path %q contains path traversal", sourcePath)
	}
	cleanDestPath := filepath.Clean(destPath)
	if strings.Contains(cleanDestPath, "..") {
		return fmt.Errorf("destination path %q contains path traversal", destPath)
	}

	destDir := filepath.Dir(cleanDestPath)
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("failed to create destination directory %q: %w", destDir, err)
	}

	sourceFile, err := os.Open(cleanSourcePath) // #nosec G304 -- Path is validated and cleaned to prevent traversal
	if err != nil {
		return fmt.Errorf("failed to open source file %q: %w", cleanSourcePath, err)
	}
	defer func() { _ = sourceFile.Close() }()

	destFile, err := os.OpenFile(cleanDestPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode) // #nosec G304
	if err != nil {
		return fmt.Errorf("failed to create destination file %q: %w", cleanDestPath, err)
	}
	defer func() { _ = destFile.Close() }()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	if err := os.Chmod(destPath, fileMode); err != nil {
		return fmt.Errorf("failed to set executable permissions on file %q: %w", destPath, err)
	}

	return nil
}
