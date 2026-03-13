//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestKustomizeCustomIdentityOverlay_RewritesOperatorIdentityFields(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(filepath.Join("..", "..", "config", "overlays"), ".tmp-kustomize-custom-identity-")
	if err != nil {
		t.Fatalf("create temp overlay dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	overlayPath := filepath.Join("..", "..", "config", "overlays", "custom-identity", "kustomization.yaml")
	kustomizationBytes, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatalf("read custom-identity overlay: %v", err)
	}
	kustomization := strings.Replace(
		string(kustomizationBytes),
		"namespace: openbao-operator-system",
		"namespace: custom-operator\nnamePrefix: demo-",
		1,
	)
	if err := os.WriteFile(filepath.Join(tmpDir, "kustomization.yaml"), []byte(kustomization), 0o600); err != nil {
		t.Fatalf("write temp kustomization: %v", err)
	}

	yamlBytes := kustomizeBuild(t, tmpDir)
	objs := parseYAMLToUnstructured(t, yamlBytes, nil)

	controller := mustFindObject(t, objs, "apps/v1", "Deployment", "demo-openbao-operator-controller")
	provisioner := mustFindObject(t, objs, "apps/v1", "Deployment", "demo-openbao-operator-provisioner")
	controllerPolicy := mustFindObject(t, objs, "admissionregistration.k8s.io/v1", "ValidatingAdmissionPolicy", "demo-openbao-operator-openbao-restrict-controller-rbac")
	provisionerPolicy := mustFindObject(t, objs, "admissionregistration.k8s.io/v1", "ValidatingAdmissionPolicy", "demo-openbao-operator-openbao-restrict-provisioner-rbac")
	tenantPolicy := mustFindObject(t, objs, "admissionregistration.k8s.io/v1", "ValidatingAdmissionPolicy", "demo-openbao-operator-openbao-validate-openbao-tenant")

	if got := envVarValue(t, controller, "OPERATOR_SERVICE_ACCOUNT_NAME"); got != "demo-openbao-operator-controller" {
		t.Fatalf("controller OPERATOR_SERVICE_ACCOUNT_NAME=%q, want %q", got, "demo-openbao-operator-controller")
	}
	if got := envVarValue(t, provisioner, "OPERATOR_SERVICE_ACCOUNT_NAME"); got != "demo-openbao-operator-controller" {
		t.Fatalf("provisioner OPERATOR_SERVICE_ACCOUNT_NAME=%q, want %q", got, "demo-openbao-operator-controller")
	}

	if got := policyVariableExpression(t, controllerPolicy, "operator_namespace"); got != "'custom-operator'" {
		t.Fatalf("controller policy operator_namespace=%q, want %q", got, "'custom-operator'")
	}
	if got := policyVariableExpression(t, controllerPolicy, "controller_serviceaccount_name"); got != "'demo-openbao-operator-controller'" {
		t.Fatalf("controller policy controller_serviceaccount_name=%q, want %q", got, "'demo-openbao-operator-controller'")
	}
	if got := policyVariableExpression(t, provisionerPolicy, "provisioner_serviceaccount_name"); got != "'demo-openbao-operator-provisioner'" {
		t.Fatalf("provisioner policy provisioner_serviceaccount_name=%q, want %q", got, "'demo-openbao-operator-provisioner'")
	}
	if got := policyVariableExpression(t, tenantPolicy, "operator_namespace"); got != "'custom-operator'" {
		t.Fatalf("tenant policy operator_namespace=%q, want %q", got, "'custom-operator'")
	}
}

func TestKustomizeSingleTenantOverlay_CustomOperatorAndTargetNamespace(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(filepath.Join("..", "..", "config", "overlays"), ".tmp-kustomize-single-tenant-")
	if err != nil {
		t.Fatalf("create temp overlay dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	sourceDir := filepath.Join("..", "..", "config", "overlays", "single-tenant")
	for _, name := range []string{
		"kustomization.yaml",
		"operator_namespace.yaml",
		"single_tenant_clusterrole.yaml",
		"target_namespace_config.yaml",
		"target_namespace_rolebinding.yaml",
	} {
		content, err := os.ReadFile(filepath.Join(sourceDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		switch name {
		case "kustomization.yaml":
			content = []byte(strings.Replace(string(content), "namespace: openbao-operator-system", "namespace: custom-operator\nnamePrefix: demo-", 1))
		case "target_namespace_config.yaml":
			content = []byte(strings.Replace(string(content), "WATCH_NAMESPACE: openbao", "WATCH_NAMESPACE: tenant-openbao", 1))
		}
		if err := os.WriteFile(filepath.Join(tmpDir, name), content, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	yamlBytes := kustomizeBuild(t, tmpDir)
	objs := parseYAMLToUnstructured(t, yamlBytes, nil)

	controller := mustFindObject(t, objs, "apps/v1", "Deployment", "demo-openbao-operator-controller")
	roleBinding := mustFindObject(t, objs, "rbac.authorization.k8s.io/v1", "RoleBinding", "demo-openbao-operator-single-tenant")
	operatorNS := mustFindObject(t, objs, "v1", "Namespace", "custom-operator")

	if operatorNS == nil {
		t.Fatal("custom operator namespace was not rendered")
	}
	if controller.GetNamespace() != "custom-operator" {
		t.Fatalf("controller namespace=%q, want %q", controller.GetNamespace(), "custom-operator")
	}
	if got := envVarValue(t, controller, "WATCH_NAMESPACE"); got != "tenant-openbao" {
		t.Fatalf("WATCH_NAMESPACE=%q, want %q", got, "tenant-openbao")
	}
	if roleBinding.GetNamespace() != "tenant-openbao" {
		t.Fatalf("rolebinding namespace=%q, want %q", roleBinding.GetNamespace(), "tenant-openbao")
	}

	subjects, found, err := unstructured.NestedSlice(roleBinding.Object, "subjects")
	if err != nil || !found || len(subjects) != 1 {
		t.Fatalf("read rolebinding subjects: found=%v len=%d err=%v", found, len(subjects), err)
	}
	subject, ok := subjects[0].(map[string]any)
	if !ok {
		t.Fatalf("rolebinding subject has unexpected type %T", subjects[0])
	}
	if got, _ := subject["name"].(string); got != "demo-openbao-operator-controller" {
		t.Fatalf("rolebinding subject name=%q, want %q", got, "demo-openbao-operator-controller")
	}
	if got, _ := subject["namespace"].(string); got != "custom-operator" {
		t.Fatalf("rolebinding subject namespace=%q, want %q", got, "custom-operator")
	}
}

func mustFindObject(t *testing.T, objs []*unstructured.Unstructured, apiVersion, kind, name string) *unstructured.Unstructured {
	t.Helper()

	for _, obj := range objs {
		if obj.GetAPIVersion() == apiVersion && obj.GetKind() == kind && obj.GetName() == name {
			return obj
		}
	}

	t.Fatalf("object %s %s %s not found", apiVersion, kind, name)
	return nil
}

func envVarValue(t *testing.T, obj *unstructured.Unstructured, name string) string {
	t.Helper()

	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		t.Fatalf("containers not found: %v", err)
	}
	for _, container := range containers {
		containerMap, ok := container.(map[string]any)
		if !ok {
			continue
		}
		if containerMap["name"] != "manager" {
			continue
		}
		envs, ok := containerMap["env"].([]any)
		if !ok {
			t.Fatalf("manager env not found")
		}
		for _, env := range envs {
			envMap, ok := env.(map[string]any)
			if !ok {
				continue
			}
			if envMap["name"] == name {
				if value, ok := envMap["value"].(string); ok {
					return value
				}
				t.Fatalf("env %s has no string value", name)
			}
		}
	}

	t.Fatalf("env %s not found", name)
	return ""
}

func policyVariableExpression(t *testing.T, obj *unstructured.Unstructured, name string) string {
	t.Helper()

	variables, found, err := unstructured.NestedSlice(obj.Object, "spec", "variables")
	if err != nil || !found {
		t.Fatalf("policy variables not found: %v", err)
	}
	for _, variable := range variables {
		variableMap, ok := variable.(map[string]any)
		if !ok {
			continue
		}
		if variableMap["name"] == name {
			if expression, ok := variableMap["expression"].(string); ok {
				return expression
			}
			t.Fatalf("variable %s has no string expression", name)
		}
	}

	t.Fatalf("policy variable %s not found", name)
	return ""
}
