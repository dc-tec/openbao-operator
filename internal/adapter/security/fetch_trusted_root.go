//go:build ignore
// +build ignore

// This is a temporary helper program to fetch the trusted_root.json file.
// It should be run via: go run internal/security/fetch_trusted_root.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
)

func main() {
	// Try to fetch using TUF with a temporary directory
	tmpDir := filepath.Join(os.TempDir(), "sigstore-tuf-fetch")
	opts := tuf.DefaultOptions()
	opts.CachePath = tmpDir

	tr, err := root.FetchTrustedRootWithOptions(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching trusted root: %v\n", err)
		fmt.Fprintf(os.Stderr, "Trying alternative method...\n")

		// Fallback: try without options
		tr, err = root.FetchTrustedRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(tr, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling to JSON: %v\n", err)
		os.Exit(1)
	}

	// Write to trusted_root.json in the same directory
	outputPath := filepath.Join("internal", "security", "trusted_root.json")
	if err := os.WriteFile(outputPath, jsonData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully fetched trusted_root.json to %s\n", outputPath)
}
