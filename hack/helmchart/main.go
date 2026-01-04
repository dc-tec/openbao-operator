// Package main provides a tool to sync CRDs to the Helm chart.
// The operator templates are now native Helm templates instead of generated.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type options struct {
	crdInputDir  string
	crdOutputDir string
}

func main() {
	var opts options
	flag.StringVar(&opts.crdInputDir, "crd-in-dir", "config/crd/bases", "Path to config/crd/bases directory")
	flag.StringVar(
		&opts.crdOutputDir,
		"crd-out-dir",
		"charts/openbao-operator/crds",
		"Path to charts/openbao-operator/crds directory",
	)
	flag.Parse()

	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	if err := syncCRDs(opts); err != nil {
		return fmt.Errorf("sync CRDs: %w", err)
	}
	return nil
}

func readLines(path string) ([]string, error) {
	// #nosec G304 -- path is constructed from a controlled input directory and file name.
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func leadingWhitespace(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return s[:i]
		}
	}
	return s
}

func syncCRDs(opts options) error {
	entries, err := os.ReadDir(opts.crdInputDir)
	if err != nil {
		return fmt.Errorf("read CRD input dir %q: %w", opts.crdInputDir, err)
	}
	if err := os.MkdirAll(opts.crdOutputDir, 0o750); err != nil {
		return fmt.Errorf("create CRD output dir %q: %w", opts.crdOutputDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}

		inPath := filepath.Join(opts.crdInputDir, name)
		lines, err := readLines(inPath)
		if err != nil {
			return fmt.Errorf("read %q: %w", inPath, err)
		}

		lines, err = ensureHelmKeepAnnotation(lines)
		if err != nil {
			return fmt.Errorf("ensure Helm keep annotation for %q: %w", inPath, err)
		}

		outPath := filepath.Join(opts.crdOutputDir, name)
		// #nosec G306 -- writes non-sensitive CRD YAML intended to be committed to the repo.
		if err := os.WriteFile(outPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			return fmt.Errorf("write %q: %w", outPath, err)
		}
	}

	return nil
}

func ensureHelmKeepAnnotation(lines []string) ([]string, error) {
	for _, line := range lines {
		if strings.TrimSpace(line) == "helm.sh/resource-policy: keep" {
			return lines, nil
		}
	}

	for i, line := range lines {
		if strings.TrimSpace(line) != "annotations:" {
			continue
		}
		indent := leadingWhitespace(line)
		insert := indent + "  helm.sh/resource-policy: keep"
		lines = append(lines[:i+1], append([]string{insert}, lines[i+1:]...)...)
		return lines, nil
	}

	return nil, fmt.Errorf("annotations block not found")
}
