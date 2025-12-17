package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func run(ctx context.Context) error {
	applyUmaskFromEnv()

	var (
		watchFile string
		interval  time.Duration
	)

	flag.StringVar(&watchFile, "watch-file", "", "Path to the file to watch for changes")
	flag.DurationVar(&interval, "interval", 10*time.Second, "Polling interval")
	flag.Parse()

	// remaining args are the command to run
	cmdArgs := flag.Args()
	if len(cmdArgs) == 0 {
		return fmt.Errorf("no command specified to run")
	}

	// 1. Start the child process
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...) // #nosec G204 -- This wrapper intentionally executes user-provided commands
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ() // Pass through environment variables

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start child process: %w", err)
	}

	// 2. Setup Signal Forwarding (OS -> Wrapper -> Child)
	// We catch SIGTERM/SIGINT to allow OpenBao to shut down gracefully (e.g. step down leader)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sig := <-sigChan:
				if err := cmd.Process.Signal(sig); err != nil {
					log.Printf("Failed to forward signal %v: %v", sig, err)
				}
			}
		}
	}()

	// 3. Start File Watcher (File Change -> Wrapper -> SIGHUP to Child)
	if watchFile != "" {
		watchCtx, watchCancel := context.WithCancel(ctx)
		defer watchCancel()
		go watchFileForChanges(watchCtx, watchFile, interval, func() {
			log.Printf("File %s changed. Sending SIGHUP to child process...", watchFile)
			if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
				log.Printf("Failed to signal child process: %v", err)
			}
		})
	}

	// 4. Wait for child to exit
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("child process exited with code %d: %w", exitErr.ExitCode(), err)
		}
		return fmt.Errorf("child process exited with error: %w", err)
	}

	return nil
}

func applyUmaskFromEnv() {
	raw := os.Getenv("UMASK")
	if raw == "" {
		return
	}

	mask, err := strconv.ParseUint(raw, 8, 32)
	if err != nil {
		log.Printf("Invalid UMASK %q (expected octal), leaving default umask unchanged", raw)
		return
	}

	syscall.Umask(int(mask))
}

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
	// Validate and clean path to prevent path traversal
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

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}
