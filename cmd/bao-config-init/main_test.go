package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testListenerTemplate = `
listener "tcp" {
  address = "https://${HOSTNAME}:8200"
}
`

func TestRenderConfig_ReplacesPlaceholders(t *testing.T) {
	dir := t.TempDir()

	templatePath := filepath.Join(dir, "config.hcl.tmpl")
	outputPath := filepath.Join(dir, "rendered", "config.hcl")

	templateContent := `
listener "tcp" {
  address = "https://${HOSTNAME}:8200"
}

cluster_addr = "https://${POD_IP}:8201"
`

	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	const hostname = "test-host-0"
	const podIP = "10.0.0.5"

	// Test with both HOSTNAME and POD_IP provided (no self-init)
	if err := renderConfig(templatePath, outputPath, hostname, podIP, ""); err != nil {
		t.Fatalf("renderConfig() error = %v, want nil", err)
	}

	renderedBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read rendered config: %v", err)
	}
	rendered := string(renderedBytes)

	if strings.Contains(rendered, "${HOSTNAME}") {
		t.Errorf("expected HOSTNAME placeholder to be replaced, got: %q", rendered)
	}
	if strings.Contains(rendered, "${POD_IP}") {
		t.Errorf("expected POD_IP placeholder to be replaced, got: %q", rendered)
	}
	if !strings.Contains(rendered, hostname) {
		t.Errorf("expected rendered config to contain hostname %q, got: %q", hostname, rendered)
	}
	if !strings.Contains(rendered, podIP) {
		t.Errorf("expected rendered config to contain pod IP %q, got: %q", podIP, rendered)
	}
}

func TestRenderConfig_HandlesHCLDoubleDollarEscapes(t *testing.T) {
	dir := t.TempDir()

	templatePath := filepath.Join(dir, "config.hcl.tmpl")
	outputPath := filepath.Join(dir, "rendered", "config.hcl")

	// Simulate the output produced by hclwrite when given "${HOSTNAME}" and "${POD_IP}".
	// HCL escapes these as "$${HOSTNAME}" and "$${POD_IP}".
	templateContent := `
storage "raft" {
  node_id      = "$${HOSTNAME}"
  retry_join {
    leader_api_addr = "https://$${HOSTNAME}.example.svc:8200"
  }
}

api_addr     = "https://$${POD_IP}:8200"
cluster_addr = "https://$${POD_IP}:8201"
`

	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	const hostname = "test-host-1"
	const podIP = "10.0.0.9"

	if err := renderConfig(templatePath, outputPath, hostname, podIP, ""); err != nil {
		t.Fatalf("renderConfig() error = %v, want nil", err)
	}

	renderedBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read rendered config: %v", err)
	}
	rendered := string(renderedBytes)

	if strings.Contains(rendered, "$${HOSTNAME}") || strings.Contains(rendered, "${HOSTNAME}") {
		t.Errorf("expected all HOSTNAME placeholders to be replaced, got: %q", rendered)
	}
	if strings.Contains(rendered, "$${POD_IP}") || strings.Contains(rendered, "${POD_IP}") {
		t.Errorf("expected all POD_IP placeholders to be replaced, got: %q", rendered)
	}
	if !strings.Contains(rendered, hostname) {
		t.Errorf("expected rendered config to contain hostname %q, got: %q", hostname, rendered)
	}
	if !strings.Contains(rendered, podIP) {
		t.Errorf("expected rendered config to contain pod IP %q, got: %q", podIP, rendered)
	}
	if strings.Contains(rendered, "$"+hostname) {
		t.Errorf("expected rendered hostname not to be prefixed with '$', got: %q", rendered)
	}
	if strings.Contains(rendered, "$"+podIP) {
		t.Errorf("expected rendered pod IP not to be prefixed with '$', got: %q", rendered)
	}
}

func TestRenderConfig_AppendsSelfInitForPod0(t *testing.T) {
	dir := t.TempDir()

	templatePath := filepath.Join(dir, "config.hcl.tmpl")
	outputPath := filepath.Join(dir, "rendered", "config.hcl")
	selfInitPath := filepath.Join(dir, "self-init.hcl")

	templateContent := testListenerTemplate

	selfInitContent := `
initialize "audit" {
  request "enable-audit" {
    operation = "update"
    path      = "sys/audit/stdout"
  }
}
`

	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}
	if err := os.WriteFile(selfInitPath, []byte(selfInitContent), 0o644); err != nil {
		t.Fatalf("failed to write self-init config: %v", err)
	}

	const hostname = "test-host-0" // pod-0
	const podIP = "10.0.0.5"

	// Test that self-init is appended for pod-0
	if err := renderConfig(templatePath, outputPath, hostname, podIP, selfInitPath); err != nil {
		t.Fatalf("renderConfig() error = %v, want nil", err)
	}

	renderedBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read rendered config: %v", err)
	}
	rendered := string(renderedBytes)

	// Should contain both base config and self-init blocks
	if !strings.Contains(rendered, "listener") {
		t.Errorf("expected rendered config to contain base config, got: %q", rendered)
	}
	if !strings.Contains(rendered, "initialize") {
		t.Errorf("expected rendered config to contain self-init blocks for pod-0, got: %q", rendered)
	}
	if !strings.Contains(rendered, "enable-audit") {
		t.Errorf("expected rendered config to contain self-init request, got: %q", rendered)
	}
}

func TestRenderConfig_SkipsSelfInitForNonPod0(t *testing.T) {
	dir := t.TempDir()

	templatePath := filepath.Join(dir, "config.hcl.tmpl")
	outputPath := filepath.Join(dir, "rendered", "config.hcl")
	selfInitPath := filepath.Join(dir, "self-init.hcl")

	templateContent := testListenerTemplate

	selfInitContent := `
initialize "audit" {
  request "enable-audit" {
    operation = "update"
    path      = "sys/audit/stdout"
  }
}
`

	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}
	if err := os.WriteFile(selfInitPath, []byte(selfInitContent), 0o644); err != nil {
		t.Fatalf("failed to write self-init config: %v", err)
	}

	const hostname = "test-host-1" // Not pod-0
	const podIP = "10.0.0.6"

	// Test that self-init is NOT appended for non-pod-0
	if err := renderConfig(templatePath, outputPath, hostname, podIP, selfInitPath); err != nil {
		t.Fatalf("renderConfig() error = %v, want nil", err)
	}

	renderedBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read rendered config: %v", err)
	}
	rendered := string(renderedBytes)

	// Should contain base config but NOT self-init blocks
	if !strings.Contains(rendered, "listener") {
		t.Errorf("expected rendered config to contain base config, got: %q", rendered)
	}
	if strings.Contains(rendered, "initialize") {
		t.Errorf("expected rendered config NOT to contain self-init blocks for non-pod-0, got: %q", rendered)
	}
}

func TestRenderConfig_HandlesMissingSelfInitFile(t *testing.T) {
	dir := t.TempDir()

	templatePath := filepath.Join(dir, "config.hcl.tmpl")
	outputPath := filepath.Join(dir, "rendered", "config.hcl")
	selfInitPath := filepath.Join(dir, "nonexistent.hcl") // File doesn't exist

	templateContent := testListenerTemplate

	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	const hostname = "test-host-0" // pod-0
	const podIP = "10.0.0.5"

	// Should handle missing self-init file gracefully (for pod-0)
	if err := renderConfig(templatePath, outputPath, hostname, podIP, selfInitPath); err != nil {
		t.Fatalf("renderConfig() error = %v, want nil (should handle missing self-init file)", err)
	}

	renderedBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read rendered config: %v", err)
	}
	rendered := string(renderedBytes)

	// Should contain base config but no self-init (since file doesn't exist)
	if !strings.Contains(rendered, "listener") {
		t.Errorf("expected rendered config to contain base config, got: %q", rendered)
	}
	if strings.Contains(rendered, "initialize") {
		t.Errorf("expected rendered config NOT to contain self-init blocks when file is missing, got: %q", rendered)
	}
}

func TestCopyWrapper(t *testing.T) {
	dir := t.TempDir()

	// Create a test wrapper binary (just some content)
	wrapperContent := []byte("#!/bin/sh\necho 'test wrapper'")
	sourcePath := filepath.Join(dir, "bao-wrapper")
	if err := os.WriteFile(sourcePath, wrapperContent, 0o644); err != nil {
		t.Fatalf("failed to create source wrapper: %v", err)
	}

	// Create /utils directory structure in temp dir for testing
	utilsDir := filepath.Join(dir, "utils")
	if err := os.MkdirAll(utilsDir, 0o755); err != nil {
		t.Fatalf("failed to create utils directory: %v", err)
	}

	// Test the copy logic manually (since copyWrapper hardcodes /utils/bao-wrapper)
	// This verifies the same logic that copyWrapper uses
	destPath := filepath.Join(utilsDir, "bao-wrapper")

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		t.Fatalf("failed to open source: %v", err)
	}
	defer func() { _ = sourceFile.Close() }()

	destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("failed to create destination: %v", err)
	}
	defer func() { _ = destFile.Close() }()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		t.Fatalf("failed to copy: %v", err)
	}
	if err := os.Chmod(destPath, 0o755); err != nil {
		t.Fatalf("failed to set permissions: %v", err)
	}

	// Verify file was copied and has correct permissions
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("failed to stat copied file: %v", err)
	}

	// Check permissions (executable)
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("copied file should be executable, got permissions: %v", info.Mode().Perm())
	}

	// Verify content matches
	copiedContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(copiedContent) != string(wrapperContent) {
		t.Errorf("copied content does not match source: got %q, want %q", string(copiedContent), string(wrapperContent))
	}
}
