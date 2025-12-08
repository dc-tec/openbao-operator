package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// configFileMode is the file mode used for rendered configuration files.
	// Configuration is not secret material, so 0644 is appropriate.
	configFileMode = 0o644
)

// renderConfig reads a template file, substitutes environment-driven placeholders,
// and writes the rendered configuration to the specified output path.
//
// Supported placeholders:
//   - ${HOSTNAME} - replaced with the provided hostname string.
//   - ${POD_IP}   - replaced with the provided podIP string.
//
// If selfInitPath is provided and hostname ends with "-0" (pod-0), the self-init
// configuration is appended to the base configuration.
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
	// POD_IP may not be available immediately when the init container runs.
	// We'll wait a short time for it to become available, as it's needed for proper config rendering.
	if strings.TrimSpace(podIP) == "" {
		// Wait briefly for POD_IP to become available (up to 5 seconds)
		for i := 0; i < 10; i++ {
			time.Sleep(500 * time.Millisecond)
			podIP = strings.TrimSpace(os.Getenv("POD_IP"))
			if podIP != "" {
				break
			}
		}
		// If still not available, fail - POD_IP is required for proper configuration
		if strings.TrimSpace(podIP) == "" {
			return fmt.Errorf(
				"POD_IP environment variable is required but not available after waiting (must be set from pod status.podIP)")
		}
	}

	templateFile, err := os.Open(templatePath)
	if err != nil {
		return fmt.Errorf("failed to open template file %q: %w", templatePath, err)
	}
	defer func() {
		_ = templateFile.Close()
	}()

	content, err := io.ReadAll(templateFile)
	if err != nil {
		return fmt.Errorf("failed to read template file %q: %w", templatePath, err)
	}

	rendered := string(content)
	// HCL escapes literal interpolation markers by doubling the leading '$',
	// so strings like "${HOSTNAME}" in the input become "$${HOSTNAME}" in the
	// rendered template. Replace both the escaped and unescaped forms to
	// ensure the final config never contains a leading '$' before the value.
	rendered = strings.ReplaceAll(rendered, "$${HOSTNAME}", hostname)
	rendered = strings.ReplaceAll(rendered, "${HOSTNAME}", hostname)
	rendered = strings.ReplaceAll(rendered, "$${POD_IP}", podIP)
	rendered = strings.ReplaceAll(rendered, "${POD_IP}", podIP)

	// If self-init is enabled and this is pod-0, append the self-init configuration
	if selfInitPath != "" && strings.HasSuffix(hostname, "-0") {
		selfInitFile, err := os.Open(selfInitPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Self-init ConfigMap may not exist if self-init is disabled
				// This is fine, just continue without it
			} else {
				return fmt.Errorf("failed to open self-init config file %q: %w", selfInitPath, err)
			}
		} else {
			defer func() {
				_ = selfInitFile.Close()
			}()

			selfInitContent, err := io.ReadAll(selfInitFile)
			if err != nil {
				return fmt.Errorf("failed to read self-init config file %q: %w", selfInitPath, err)
			}

			if len(selfInitContent) > 0 {
				// Append self-init blocks to the base config
				rendered += "\n\n" + string(selfInitContent)
			}
		}
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %q: %w", dir, err)
	}

	if err := os.WriteFile(outputPath, []byte(rendered), configFileMode); err != nil {
		return fmt.Errorf("failed to write rendered config to %q: %w", outputPath, err)
	}

	// Verify that all placeholders were actually replaced.
	// Both HOSTNAME and POD_IP are critical for proper OpenBao configuration.
	if strings.Contains(rendered, "${HOSTNAME}") {
		return fmt.Errorf("rendered config still contains ${HOSTNAME} placeholder - HOSTNAME expansion failed")
	}
	if strings.Contains(rendered, "${POD_IP}") {
		return fmt.Errorf("rendered config still contains ${POD_IP} placeholder - POD_IP expansion failed")
	}

	return nil
}

func main() {
	templatePath := flag.String("template", "", "path to the config template file")
	outputPath := flag.String("output", "", "path to write the rendered config file")
	selfInitPath := flag.String("self-init", "", "optional path to self-init config file (only used for pod-0)")
	wrapperSource := flag.String("copy-wrapper", "", "optional path to wrapper binary to copy to /utils/bao-wrapper")
	probeSource := flag.String("copy-probe", "", "optional path to probe binary to copy to /utils/bao-probe")
	flag.Parse()

	// Copy wrapper binary if specified (before rendering config)
	if *wrapperSource != "" {
		if err := copyWrapper(*wrapperSource); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "bao-config-init error: failed to copy wrapper: %v\n", err)
			os.Exit(1)
		}
	}

	if *probeSource != "" {
		if err := copyProbe(*probeSource); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "bao-config-init error: failed to copy probe: %v\n", err)
			os.Exit(1)
		}
	}

	if err := renderConfig(
		*templatePath,
		*outputPath,
		os.Getenv("HOSTNAME"),
		os.Getenv("POD_IP"),
		*selfInitPath,
	); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bao-config-init error: %v\n", err)
		os.Exit(1)
	}
}

// copyWrapper copies the wrapper binary from source to /utils/bao-wrapper
// and sets executable permissions. This eliminates the need for shell commands
// in the init container, allowing it to use a distroless/static image (no shell).
func copyWrapper(sourcePath string) error {
	const (
		wrapperDestPath = "/utils/bao-wrapper"
		wrapperFileMode = 0o755 // Executable permissions
	)

	// Ensure destination directory exists
	destDir := filepath.Dir(wrapperDestPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory %q: %w", destDir, err)
	}

	// Open source file
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source wrapper file %q: %w", sourcePath, err)
	}
	defer sourceFile.Close()

	// Create destination file
	destFile, err := os.OpenFile(wrapperDestPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, wrapperFileMode)
	if err != nil {
		return fmt.Errorf("failed to create destination wrapper file %q: %w", wrapperDestPath, err)
	}
	defer destFile.Close()

	// Copy file contents
	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy wrapper binary: %w", err)
	}

	// Ensure executable permissions (even though we set them on OpenFile, chmod for safety)
	if err := os.Chmod(wrapperDestPath, wrapperFileMode); err != nil {
		return fmt.Errorf("failed to set executable permissions on wrapper: %w", err)
	}

	return nil
}

func copyProbe(sourcePath string) error {
	const (
		probeDestPath = "/utils/bao-probe"
		probeFileMode = 0o755 // Executable permissions
	)

	destDir := filepath.Dir(probeDestPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory %q: %w", destDir, err)
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source probe file %q: %w", sourcePath, err)
	}
	defer sourceFile.Close()

	destFile, err := os.OpenFile(probeDestPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, probeFileMode)
	if err != nil {
		return fmt.Errorf("failed to create destination probe file %q: %w", probeDestPath, err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy probe binary: %w", err)
	}

	if err := os.Chmod(probeDestPath, probeFileMode); err != nil {
		return fmt.Errorf("failed to set executable permissions on probe: %w", err)
	}

	return nil
}
