package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// renderConfig reads a template file, substitutes environment-driven placeholders,
// and writes the rendered configuration to the specified output path.
func renderConfig(templatePath, outputPath, hostname, podIP, selfInitPath string) error {
	if strings.TrimSpace(templatePath) == "" {
		return fmt.Errorf("template path is required")
	}
	if strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("output path is required")
	}
	if strings.TrimSpace(hostname) == "" {
		return fmt.Errorf("HOSTNAME environment variable is required (must be set from pod metadata.name)")
	}

	resolvedPodIP, err := waitForPodIP(podIP)
	if err != nil {
		return err
	}

	content, err := readValidatedFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to open template file %q: %w", templatePath, err)
	}

	rendered := replacePlaceholders(string(content), hostname, resolvedPodIP)
	if selfInitPath != "" && strings.HasSuffix(hostname, "-0") {
		rendered, err = appendSelfInitConfig(rendered, selfInitPath)
		if err != nil {
			return err
		}
	}

	if err := writeRenderedConfig(outputPath, rendered); err != nil {
		return err
	}

	if strings.Contains(rendered, "${HOSTNAME}") {
		return fmt.Errorf("rendered config still contains ${HOSTNAME} placeholder - HOSTNAME expansion failed")
	}
	if strings.Contains(rendered, "${POD_IP}") {
		return fmt.Errorf("rendered config still contains ${POD_IP} placeholder - POD_IP expansion failed")
	}

	return nil
}

func waitForPodIP(podIP string) (string, error) {
	if strings.TrimSpace(podIP) != "" {
		return podIP, nil
	}

	pollCtx := context.Background()
	pollFn := func(ctx context.Context) (bool, error) {
		podIP = strings.TrimSpace(os.Getenv(envPodIP))
		return podIP != "", nil
	}
	err := wait.PollUntilContextTimeout(pollCtx, 500*time.Millisecond, 5*time.Second, true, pollFn)
	if strings.TrimSpace(podIP) != "" {
		return podIP, nil
	}
	if err != nil {
		return "", fmt.Errorf(
			"POD_IP environment variable is required but not available after waiting (must be set from pod status.podIP): %w",
			err,
		)
	}

	return "", fmt.Errorf("POD_IP environment variable is required but not available after waiting (must be set from pod status.podIP)")
}

func replacePlaceholders(content, hostname, podIP string) string {
	rendered := strings.ReplaceAll(content, "$${HOSTNAME}", hostname)
	rendered = strings.ReplaceAll(rendered, "${HOSTNAME}", hostname)
	rendered = strings.ReplaceAll(rendered, "$${POD_IP}", podIP)
	rendered = strings.ReplaceAll(rendered, "${POD_IP}", podIP)
	return rendered
}

func appendSelfInitConfig(rendered, selfInitPath string) (string, error) {
	selfInitContent, err := readOptionalValidatedFile(selfInitPath)
	if err != nil {
		return "", fmt.Errorf("failed to open self-init config file %q: %w", selfInitPath, err)
	}
	if len(selfInitContent) == 0 {
		return rendered, nil
	}

	return rendered + "\n\n" + string(selfInitContent), nil
}

func writeRenderedConfig(outputPath, rendered string) error {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create output directory %q: %w", dir, err)
	}

	if err := os.WriteFile(outputPath, []byte(rendered), configFileMode); err != nil {
		return fmt.Errorf("failed to write rendered config to %q: %w", outputPath, err)
	}

	return nil
}

func readValidatedFile(path string) ([]byte, error) {
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("path %q contains path traversal", path)
	}

	f, err := os.Open(cleanPath) // #nosec G304 -- Path is validated and cleaned to prevent traversal
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	return io.ReadAll(f)
}

func readOptionalValidatedFile(path string) ([]byte, error) {
	content, err := readValidatedFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	return content, nil
}
