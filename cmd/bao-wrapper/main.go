package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
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
		log.Fatal("No command specified to run")
	}

	// 1. Start the child process
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ() // Pass through environment variables

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start child process: %v", err)
	}

	// 2. Setup Signal Forwarding (OS -> Wrapper -> Child)
	// We catch SIGTERM/SIGINT to allow OpenBao to shut down gracefully (e.g. step down leader)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigChan {
			if err := cmd.Process.Signal(sig); err != nil {
				log.Printf("Failed to forward signal %v: %v", sig, err)
			}
		}
	}()

	// 3. Start File Watcher (File Change -> Wrapper -> SIGHUP to Child)
	if watchFile != "" {
		go func() {
			lastHash, _ := getFileHash(watchFile)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for range ticker.C {
				currentHash, err := getFileHash(watchFile)
				if err != nil {
					log.Printf("Error reading watch file: %v", err)
					continue
				}

				if len(lastHash) > 0 && !bytes.Equal(lastHash, currentHash) {
					log.Printf("File %s changed. Sending SIGHUP to child process...", watchFile)
					if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
						log.Printf("Failed to signal child process: %v", err)
					} else {
						lastHash = currentHash
					}
				} else if len(lastHash) == 0 {
					lastHash = currentHash
				}
			}
		}()
	}

	// 4. Wait for child to exit
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatalf("Child process exited with error: %v", err)
	}
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

func getFileHash(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}
