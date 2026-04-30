package main

import (
	"os"
	"path/filepath"
	"testing"
)

const testModulePath = "example.com/operator"

func TestAnalyzePackagesFindsSamePackageTestOnlyExport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFile(t, dir, "helper.go", `package app

func ProductionHelper() {}

func TestOnlyHelper() {}

func useProductionHelper() {
	ProductionHelper()
}
`)
	writeGoFile(t, dir, "helper_test.go", `package app

import "testing"

func TestHelper(t *testing.T) {
	TestOnlyHelper()
}
`)

	findings, err := analyzePackages(testModulePath, []packageInfo{
		{
			ImportPath:  testModulePath + "/internal/app",
			Dir:         dir,
			GoFiles:     []string{"helper.go"},
			TestGoFiles: []string{"helper_test.go"},
		},
	})
	if err != nil {
		t.Fatalf("analyzePackages() error = %v", err)
	}
	requireFinding(t, findings, testModulePath+"/internal/app", "TestOnlyHelper")
	requireNoFinding(t, findings, testModulePath+"/internal/app", "ProductionHelper")
}

func TestAnalyzePackagesFindsImportedTestOnlyExport(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	adapterDir := filepath.Join(root, "adapter")
	appDir := filepath.Join(root, "app")
	writeGoFile(t, adapterDir, "client.go", `package adapter

func FakeClient() {}
`)
	writeGoFile(t, appDir, "client_test.go", `package app

import (
	"testing"

	"example.com/operator/internal/adapter"
)

func TestClient(t *testing.T) {
	adapter.FakeClient()
}
`)

	findings, err := analyzePackages(testModulePath, []packageInfo{
		{
			ImportPath: testModulePath + "/internal/adapter",
			Dir:        adapterDir,
			GoFiles:    []string{"client.go"},
		},
		{
			ImportPath:  testModulePath + "/internal/app",
			Dir:         appDir,
			TestGoFiles: []string{"client_test.go"},
		},
	})
	if err != nil {
		t.Fatalf("analyzePackages() error = %v", err)
	}
	requireFinding(t, findings, testModulePath+"/internal/adapter", "FakeClient")
}

func TestAnalyzePackagesFindsExternalTestPackageExport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFile(t, dir, "client.go", `package adapter

func FakeClient() {}
`)
	writeGoFile(t, dir, "client_external_test.go", `package adapter_test

import (
	"testing"

	"example.com/operator/internal/adapter"
)

func TestClient(t *testing.T) {
	adapter.FakeClient()
}
`)

	findings, err := analyzePackages(testModulePath, []packageInfo{
		{
			ImportPath:   testModulePath + "/internal/adapter",
			Dir:          dir,
			GoFiles:      []string{"client.go"},
			XTestGoFiles: []string{"client_external_test.go"},
		},
	})
	if err != nil {
		t.Fatalf("analyzePackages() error = %v", err)
	}
	requireFinding(t, findings, testModulePath+"/internal/adapter", "FakeClient")
}

func TestAnalyzePackagesAcceptsProductionReferences(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	adapterDir := filepath.Join(root, "adapter")
	appDir := filepath.Join(root, "app")
	writeGoFile(t, adapterDir, "client.go", `package adapter

func ClientFactory() {}
`)
	writeGoFile(t, appDir, "client.go", `package app

import "example.com/operator/internal/adapter"

func BuildClient() {
	adapter.ClientFactory()
}
`)
	writeGoFile(t, appDir, "client_test.go", `package app

import (
	"testing"

	"example.com/operator/internal/adapter"
)

func TestClient(t *testing.T) {
	adapter.ClientFactory()
}
`)

	findings, err := analyzePackages(testModulePath, []packageInfo{
		{
			ImportPath: testModulePath + "/internal/adapter",
			Dir:        adapterDir,
			GoFiles:    []string{"client.go"},
		},
		{
			ImportPath:  testModulePath + "/internal/app",
			Dir:         appDir,
			GoFiles:     []string{"client.go"},
			TestGoFiles: []string{"client_test.go"},
		},
	})
	if err != nil {
		t.Fatalf("analyzePackages() error = %v", err)
	}
	requireNoFinding(t, findings, testModulePath+"/internal/adapter", "ClientFactory")
}

func TestAnalyzePackagesSkipsTestutilPackages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFile(t, dir, "mock.go", `package testutil

type MockRuntime struct{}
`)
	writeGoFile(t, dir, "mock_test.go", `package testutil

import "testing"

func TestMockRuntime(t *testing.T) {
	_ = MockRuntime{}
}
`)

	findings, err := analyzePackages(testModulePath, []packageInfo{
		{
			ImportPath:  testModulePath + "/internal/platform/testutil",
			Dir:         dir,
			GoFiles:     []string{"mock.go"},
			TestGoFiles: []string{"mock_test.go"},
		},
	})
	if err != nil {
		t.Fatalf("analyzePackages() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func writeGoFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create temp package dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write temp Go file: %v", err)
	}
}

func requireFinding(t *testing.T, findings []finding, importPath, name string) {
	t.Helper()

	for _, finding := range findings {
		if finding.Decl.Key.ImportPath == importPath && finding.Decl.Key.Name == name {
			return
		}
	}
	t.Fatalf("missing finding for %s.%s in %#v", importPath, name, findings)
}

func requireNoFinding(t *testing.T, findings []finding, importPath, name string) {
	t.Helper()

	for _, finding := range findings {
		if finding.Decl.Key.ImportPath == importPath && finding.Decl.Key.Name == name {
			t.Fatalf("unexpected finding for %s.%s in %#v", importPath, name, findings)
		}
	}
}
