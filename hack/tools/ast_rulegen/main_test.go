package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportRegex(t *testing.T) {
	t.Parallel()

	regex, err := importRegex(
		"github.com/dc-tec/openbao-operator",
		[]string{"internal/service/upgrade", "internal/adapter/revision", "internal/service/upgrade"},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("importRegex returned error: %v", err)
	}

	expected := strings.Join([]string{
		`"github\.com/dc-tec/openbao-operator/(internal/adapter/revision(/[^"]*)?|`,
		`internal/service/upgrade(/[^"]*)?)"`,
	}, "")
	if regex != expected {
		t.Fatalf("unexpected regex:\nwant: %s\ngot:  %s", expected, regex)
	}
}

func TestImportRegexWithExternalImports(t *testing.T) {
	t.Parallel()

	regex, err := importRegex(
		"github.com/dc-tec/openbao-operator",
		nil,
		[]string{"sigs.k8s.io/controller-runtime/pkg/reconcile"},
		[]string{"sigs.k8s.io/controller-runtime"},
	)
	if err != nil {
		t.Fatalf("importRegex returned error: %v", err)
	}

	expected := `"(sigs\.k8s\.io/controller-runtime/pkg/reconcile(/[^"]*)?|sigs\.k8s\.io/controller-runtime)"`
	if regex != expected {
		t.Fatalf("unexpected regex:\nwant: %s\ngot:  %s", expected, regex)
	}
}

func TestImportRegexWithMixedImports(t *testing.T) {
	t.Parallel()

	regex, err := importRegex(
		"github.com/dc-tec/openbao-operator",
		[]string{"internal/service/upgrade"},
		[]string{"sigs.k8s.io/controller-runtime/pkg/reconcile"},
		[]string{"sigs.k8s.io/controller-runtime"},
	)
	if err != nil {
		t.Fatalf("importRegex returned error: %v", err)
	}

	expected := strings.Join([]string{
		`"(github\.com/dc-tec/openbao-operator/internal/service/upgrade(/[^"]*)?|`,
		`sigs\.k8s\.io/controller-runtime/pkg/reconcile(/[^"]*)?|`,
		`sigs\.k8s\.io/controller-runtime)"`,
	}, "")
	if regex != expected {
		t.Fatalf("unexpected regex:\nwant: %s\ngot:  %s", expected, regex)
	}
}

func TestAppSubpackageRegex(t *testing.T) {
	t.Parallel()

	regex, err := appSubpackageRegex(
		"github.com/dc-tec/openbao-operator",
		"internal/app/openbaocluster",
	)
	if err != nil {
		t.Fatalf("appSubpackageRegex returned error: %v", err)
	}

	expected := `"github\.com/dc-tec/openbao-operator/internal/app/openbaocluster/.+"`
	if regex != expected {
		t.Fatalf("unexpected regex:\nwant: %s\ngot:  %s", expected, regex)
	}
}

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	got := sanitizeName("OpenBao Cluster")
	if got != "openbao-cluster" {
		t.Fatalf("unexpected sanitized name: %s", got)
	}
}

func TestDifferenceRoots(t *testing.T) {
	t.Parallel()

	got := differenceRoots(
		[]string{"internal/service/certs", "internal/service/upgrade", "internal/service/networking"},
		[]string{"internal/service/certs"},
	)

	want := []string{"internal/service/networking", "internal/service/upgrade"}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: want %d got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected entry at %d: want %s got %s", i, want[i], got[i])
		}
	}
}

func TestVerifyLayerCoverage(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "internal")
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatalf("create app dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "controller"), 0o755); err != nil {
		t.Fatalf("create controller dir: %v", err)
	}

	policy := architecturePolicy{
		LayerCoverage: layerCoverage{
			Root: root,
			Layers: map[string][]string{
				"app":        {"app"},
				"controller": {"controller"},
			},
		},
	}

	if err := verifyLayerCoverage(policy); err != nil {
		t.Fatalf("verifyLayerCoverage returned error: %v", err)
	}
}

func TestVerifyLayerCoverageWithGroupedPackages(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "internal")
	for _, rel := range []string{
		"app",
		"controller",
		"port",
		"service/upgrade",
		"service/restore",
		"adapter/config",
		"adapter/openbao",
		"platform/constants",
		"platform/reconcile",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatalf("create %s dir: %v", rel, err)
		}
	}

	policy := architecturePolicy{
		LayerCoverage: layerCoverage{
			Root: root,
			Layers: map[string][]string{
				"app":        {"app"},
				"controller": {"controller"},
				"port":       {"port"},
				"service":    {"service/restore", "service/upgrade"},
				"adapter":    {"adapter/config", "adapter/openbao"},
				"platform":   {"platform/constants", "platform/reconcile"},
			},
		},
	}

	if err := verifyLayerCoverage(policy); err != nil {
		t.Fatalf("verifyLayerCoverage returned error: %v", err)
	}
}

func TestVerifyLayerCoverageRejectsDeepPath(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "internal")
	if err := os.MkdirAll(filepath.Join(root, "platform", "testutil", "robustness"), 0o755); err != nil {
		t.Fatalf("create nested dir: %v", err)
	}

	policy := architecturePolicy{
		LayerCoverage: layerCoverage{
			Root: root,
			Layers: map[string][]string{
				"platform": {"platform/testutil/robustness"},
			},
		},
	}

	err := verifyLayerCoverage(policy)
	if err == nil {
		t.Fatalf("expected verifyLayerCoverage to reject deep grouped paths")
	}
	if !strings.Contains(err.Error(), "internal/<pkg> or internal/<group>/<pkg>") {
		t.Fatalf("expected depth error, got: %v", err)
	}
}

func TestVerifyControllerCoverageMissingPolicyEntry(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "internal", "controller")
	if err := os.MkdirAll(filepath.Join(root, "openbaocluster"), 0o755); err != nil {
		t.Fatalf("create openbaocluster dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "newcontroller"), 0o755); err != nil {
		t.Fatalf("create newcontroller dir: %v", err)
	}

	policy := architecturePolicy{
		ControllerCoverage: controllerCoverage{Root: root},
		ControllerBoundaries: []controllerBoundary{
			{Name: "openbaocluster"},
		},
	}

	err := verifyControllerCoverage(policy)
	if err == nil {
		t.Fatalf("expected verifyControllerCoverage to fail for missing controller boundary entry")
	}
	if !strings.Contains(err.Error(), "newcontroller") {
		t.Fatalf("expected error to mention missing controller package, got: %v", err)
	}
}

func TestVerifyControllerCoverageUnknownConfiguredController(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "internal", "controller")
	if err := os.MkdirAll(filepath.Join(root, "openbaocluster"), 0o755); err != nil {
		t.Fatalf("create openbaocluster dir: %v", err)
	}

	policy := architecturePolicy{
		ControllerCoverage: controllerCoverage{Root: root},
		ControllerBoundaries: []controllerBoundary{
			{Name: "openbaocluster"},
			{Name: "ghostcontroller"},
		},
	}

	err := verifyControllerCoverage(policy)
	if err == nil {
		t.Fatalf("expected verifyControllerCoverage to fail for unknown configured controller")
	}
	if !strings.Contains(err.Error(), "ghostcontroller") {
		t.Fatalf("expected error to mention unknown configured controller, got: %v", err)
	}
}

func TestValidatePolicyGlobalBoundaryExternalOnly(t *testing.T) {
	t.Parallel()

	policy := architecturePolicy{
		ModulePath:         "github.com/dc-tec/openbao-operator",
		ServiceImportRoots: []string{"internal/service/networking"},
		AdapterImportRoots: []string{"internal/adapter/kube"},
		GlobalImportBoundaries: []globalImportBoundary{
			{
				ID:                           "external-only",
				Message:                      "no external import",
				Files:                        []string{"internal/app/**/*.go"},
				DisallowExternalExactImports: []string{"sigs.k8s.io/controller-runtime"},
			},
		},
	}

	if err := validatePolicy(policy); err != nil {
		t.Fatalf("validatePolicy returned error: %v", err)
	}
}

func TestValidatePolicyGlobalBoundaryMissingDisallowLists(t *testing.T) {
	t.Parallel()

	policy := architecturePolicy{
		ModulePath:         "github.com/dc-tec/openbao-operator",
		ServiceImportRoots: []string{"internal/service/networking"},
		AdapterImportRoots: []string{"internal/adapter/kube"},
		GlobalImportBoundaries: []globalImportBoundary{
			{
				ID:      "missing-disallow",
				Message: "missing disallow lists",
				Files:   []string{"internal/app/**/*.go"},
			},
		},
	}

	err := validatePolicy(policy)
	if err == nil {
		t.Fatalf("expected validatePolicy to fail for missing disallow lists")
	}
	if !strings.Contains(err.Error(), "must define at least one") {
		t.Fatalf("expected error to mention missing disallow lists, got: %v", err)
	}
}

func TestValidatePolicyServiceAndAppBoundaries(t *testing.T) {
	t.Parallel()

	policy := architecturePolicy{
		ModulePath: "github.com/dc-tec/openbao-operator",
		ServiceImportRoots: []string{
			"internal/service/backup",
			"internal/service/networking",
			"internal/service/opslifecycle",
		},
		AdapterImportRoots: []string{"internal/adapter/kube"},
		ServiceBoundaries: []serviceBoundary{
			{
				Name:         "backup",
				PackageRoot:  "internal/service/backup",
				Files:        []string{"internal/service/backup/**/*.go"},
				AllowService: []string{"internal/service/opslifecycle"},
			},
		},
		AppBoundaries: []appBoundary{
			{
				Name:         "openbaocluster",
				Files:        []string{"internal/app/openbaocluster/**/*.go"},
				AllowService: []string{"internal/service/networking"},
			},
		},
	}

	if err := validatePolicy(policy); err != nil {
		t.Fatalf("validatePolicy returned error: %v", err)
	}
}

func TestValidatePolicyRejectsUnknownServiceBoundaryRoot(t *testing.T) {
	t.Parallel()

	policy := architecturePolicy{
		ModulePath:         "github.com/dc-tec/openbao-operator",
		ServiceImportRoots: []string{"internal/service/backup"},
		AdapterImportRoots: []string{"internal/adapter/kube"},
		ServiceBoundaries: []serviceBoundary{
			{
				Name:        "backup",
				PackageRoot: "internal/service/ghost",
				Files:       []string{"internal/service/backup/**/*.go"},
			},
		},
	}

	err := validatePolicy(policy)
	if err == nil {
		t.Fatalf("expected validatePolicy to fail for unknown packageRoot")
	}
	if !strings.Contains(err.Error(), "serviceBoundaries[backup].packageRoot") {
		t.Fatalf("expected error to mention service boundary packageRoot, got: %v", err)
	}
}

func TestBuildRuleSpecsServiceAndAppBoundaries(t *testing.T) {
	t.Parallel()

	policy := architecturePolicy{
		ModulePath: "github.com/dc-tec/openbao-operator",
		ServiceImportRoots: []string{
			"internal/service/backup",
			"internal/service/networking",
			"internal/service/opslifecycle",
			"internal/service/upgrade",
			"internal/service/upgrade/bluegreen",
			"internal/service/upgrade/rolling",
		},
		AdapterImportRoots: []string{
			"internal/adapter/auth",
			"internal/adapter/kube",
			"internal/adapter/security",
		},
		ServiceBoundaries: []serviceBoundary{
			{
				Name:         "backup",
				DisplayName:  "Backup",
				PackageRoot:  "internal/service/backup",
				Files:        []string{"internal/service/backup/**/*.go"},
				Ignores:      []string{"**/*_test.go"},
				AllowService: []string{"internal/service/opslifecycle"},
				AllowAdapter: []string{"internal/adapter/kube"},
			},
		},
		AppBoundaries: []appBoundary{
			{
				Name:        "openbaocluster",
				DisplayName: "OpenBaoCluster",
				Files:       []string{"internal/app/openbaocluster/**/*.go"},
				Ignores:     []string{"**/*_test.go"},
				AllowService: []string{
					"internal/service/backup",
					"internal/service/networking",
					"internal/service/upgrade/bluegreen",
					"internal/service/upgrade/rolling",
				},
			},
		},
	}

	specs, err := buildRuleSpecs(policy)
	if err != nil {
		t.Fatalf("buildRuleSpecs returned error: %v", err)
	}

	want := map[string]string{
		"no-backup-service-unapproved-service-imports": strings.Join([]string{
			`"github\.com/dc-tec/openbao-operator/(internal/service/networking(/[^"]*)?|`,
			`internal/service/upgrade(/[^"]*)?|`,
			`internal/service/upgrade/bluegreen(/[^"]*)?|`,
			`internal/service/upgrade/rolling(/[^"]*)?)"`,
		}, ""),
		"no-backup-service-unapproved-adapter-imports": strings.Join([]string{
			`"github\.com/dc-tec/openbao-operator/(internal/adapter/auth(/[^"]*)?|`,
			`internal/adapter/security(/[^"]*)?)"`,
		}, ""),
		"no-openbaocluster-app-unapproved-service-imports": strings.Join([]string{
			`"github\.com/dc-tec/openbao-operator/(internal/service/opslifecycle(/[^"]*)?|`,
			`internal/service/upgrade(/[^"]*)?)"`,
		}, ""),
	}

	if len(specs) != len(want) {
		t.Fatalf("unexpected number of rule specs: want %d got %d", len(want), len(specs))
	}

	for _, spec := range specs {
		expectedRegex, ok := want[spec.ID]
		if !ok {
			t.Fatalf("unexpected rule spec ID: %s", spec.ID)
		}
		if spec.Regex != expectedRegex {
			t.Fatalf("unexpected regex for %s:\nwant: %s\ngot:  %s", spec.ID, expectedRegex, spec.Regex)
		}
	}
}

func TestValidatePolicyModuleBoundaries(t *testing.T) {
	t.Parallel()

	policy := architecturePolicy{
		ModulePath:         "github.com/dc-tec/openbao-operator",
		ServiceImportRoots: []string{"internal/service/placement"},
		AdapterImportRoots: []string{"internal/adapter/kube"},
		Modules: []moduleBoundary{
			{
				Name:        "serviceclaims",
				DisplayName: "Service Claims",
				MayDependOn: []string{
					"core",
				},
				Files: []string{"internal/app/openbaoclusterclaim/**/*.go"},
				DisallowImports: []string{
					"internal/service/placement",
				},
			},
			{
				Name: "core",
			},
		},
	}

	if err := validatePolicy(policy); err != nil {
		t.Fatalf("validatePolicy returned error: %v", err)
	}
}

func TestValidatePolicyModuleBoundaryRejectsMissingFiles(t *testing.T) {
	t.Parallel()

	policy := architecturePolicy{
		ModulePath:         "github.com/dc-tec/openbao-operator",
		ServiceImportRoots: []string{"internal/service/placement"},
		AdapterImportRoots: []string{"internal/adapter/kube"},
		Modules: []moduleBoundary{
			{
				Name: "serviceclaims",
				DisallowAPISymbols: []string{
					"RemoteRef",
				},
			},
		},
	}

	err := validatePolicy(policy)
	if err == nil {
		t.Fatalf("expected validatePolicy to fail for module boundary without files")
	}
	if !strings.Contains(err.Error(), "modules[serviceclaims].files") {
		t.Fatalf("expected error to mention missing module files, got: %v", err)
	}
}

func TestValidatePolicyModuleBoundaryRejectsUnknownDependency(t *testing.T) {
	t.Parallel()

	policy := architecturePolicy{
		ModulePath:         "github.com/dc-tec/openbao-operator",
		ServiceImportRoots: []string{"internal/service/placement"},
		AdapterImportRoots: []string{"internal/adapter/kube"},
		Modules: []moduleBoundary{
			{
				Name:        "serviceclaims",
				MayDependOn: []string{"core"},
			},
		},
	}

	err := validatePolicy(policy)
	if err == nil {
		t.Fatalf("expected validatePolicy to fail for unknown module dependency")
	}
	if !strings.Contains(err.Error(), "unknown module") {
		t.Fatalf("expected error to mention unknown module dependency, got: %v", err)
	}
}

func TestBuildRuleSpecsModuleBoundaries(t *testing.T) {
	t.Parallel()

	policy := architecturePolicy{
		ModulePath: "github.com/dc-tec/openbao-operator",
		Modules: []moduleBoundary{
			{
				Name:        "serviceclaims",
				DisplayName: "Service Claims",
				Files:       []string{"internal/app/openbaoclusterclaim/**/*.go"},
				Ignores:     []string{"**/*_test.go"},
				DisallowImports: []string{
					"internal/service/remoteexecution",
					"internal/service/placement",
				},
				DisallowAPISymbols: []string{
					"TargetClusterRef",
					"RemoteRef",
				},
				DisallowAPISymbolPatterns: []string{
					"ManagedCluster.*",
				},
			},
		},
	}

	specs, err := buildRuleSpecs(policy)
	if err != nil {
		t.Fatalf("buildRuleSpecs returned error: %v", err)
	}

	want := map[string]struct {
		kind  string
		regex string
	}{
		"no-serviceclaims-module-forbidden-imports": {
			kind: "import_spec",
			regex: strings.Join([]string{
				`"github\.com/dc-tec/openbao-operator/(internal/service/placement(/[^"]*)?|`,
				`internal/service/remoteexecution(/[^"]*)?)"`,
			}, ""),
		},
		"no-serviceclaims-module-forbidden-api-type-identifiers": {
			kind:  "type_identifier",
			regex: `^(RemoteRef|TargetClusterRef|ManagedCluster.*)$`,
		},
		"no-serviceclaims-module-forbidden-api-field-identifiers": {
			kind:  "field_identifier",
			regex: `^(RemoteRef|TargetClusterRef|ManagedCluster.*)$`,
		},
	}

	if len(specs) != len(want) {
		t.Fatalf("unexpected number of rule specs: want %d got %d", len(want), len(specs))
	}

	for _, spec := range specs {
		expected, ok := want[spec.ID]
		if !ok {
			t.Fatalf("unexpected rule spec ID: %s", spec.ID)
		}
		if spec.Kind != expected.kind {
			t.Fatalf("unexpected kind for %s: want %s got %s", spec.ID, expected.kind, spec.Kind)
		}
		if spec.Regex != expected.regex {
			t.Fatalf("unexpected regex for %s:\nwant: %s\ngot:  %s", spec.ID, expected.regex, spec.Regex)
		}
	}
}
