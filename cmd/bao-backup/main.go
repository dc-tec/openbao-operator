package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const (
	// Exit codes
	exitSuccess           = 0
	exitConfigError       = 1
	exitAuthError         = 2
	exitLeaderDiscovery   = 3
	exitSnapshotError     = 4
	exitStorageError      = 5
	exitVerificationError = 6
)

func main() {
	ctx := context.Background()

	// Check executor mode
	mode := os.Getenv("EXECUTOR_MODE")
	var err error

	switch mode {
	case "restore":
		err = runRestore(ctx)
	case "backup", "":
		// Default to backup mode for backward compatibility
		err = run(ctx)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "unknown EXECUTOR_MODE: %s (expected 'backup' or 'restore')\n", mode)
		os.Exit(exitConfigError)
	}

	if err != nil {
		prefix := "bao-backup"
		if mode == "restore" {
			prefix = "bao-restore"
		}
		_, _ = fmt.Fprintf(os.Stderr, "%s error: %v\n", prefix, err)
		os.Exit(exitCodeForError(err))
	}
	os.Exit(exitSuccess)
}

func exitCodeForError(err error) int {
	if err == nil {
		return exitSuccess
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "failed to load configuration"):
		return exitConfigError
	case strings.Contains(errStr, "failed to authenticate"):
		return exitAuthError
	case strings.Contains(errStr, "failed to find leader"):
		return exitLeaderDiscovery
	case strings.Contains(errStr, "failed to get snapshot") ||
		strings.Contains(errStr, "failed to restore snapshot"):
		return exitSnapshotError
	case strings.Contains(errStr, "failed to upload backup") ||
		strings.Contains(errStr, "failed to download snapshot") ||
		strings.Contains(errStr, "failed to create storage client"):
		return exitStorageError
	case strings.Contains(errStr, "failed to verify"):
		return exitVerificationError
	default:
		return exitConfigError
	}
}
