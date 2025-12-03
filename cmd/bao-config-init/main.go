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
	flag.Parse()

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
