//go:build integration
// +build integration

package integration

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
)

func TestKustomizeClusterScopedResourcesHaveNoNamespace(t *testing.T) {
	testCases := []struct {
		name string
		dir  string
	}{
		{
			name: "config-default",
			dir:  filepath.Join("..", "..", "config", "default"),
		},
		{
			name: "config-overlays-single-tenant",
			dir:  filepath.Join("..", "..", "config", "overlays", "single-tenant"),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			yamlBytes := kustomizeBuild(t, tc.dir)
			decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), 4096)

			for {
				var raw map[string]any
				if err := decoder.Decode(&raw); err != nil {
					if errors.Is(err, io.EOF) {
						return
					}
					t.Fatalf("decode YAML: %v", err)
				}
				if len(raw) == 0 {
					continue
				}

				obj := &unstructured.Unstructured{Object: raw}
				if obj.GetAPIVersion() == "" || obj.GetKind() == "" || obj.GetName() == "" {
					continue
				}

				if !isClusterScopedManifestObject(obj.GroupVersionKind()) {
					continue
				}

				if tc.name == "config-overlays-single-tenant" && allowsClusterScopedNamespaceInSingleTenantOverlay(obj.GroupVersionKind()) {
					continue
				}

				if obj.GetNamespace() != "" {
					t.Fatalf("cluster-scoped %s %s has unexpected namespace %q", obj.GetKind(), obj.GetName(), obj.GetNamespace())
				}
			}
		})
	}
}

func allowsClusterScopedNamespaceInSingleTenantOverlay(gvk schema.GroupVersionKind) bool {
	return gvk.Group == "admissionregistration.k8s.io" &&
		(gvk.Kind == "ValidatingAdmissionPolicy" || gvk.Kind == "ValidatingAdmissionPolicyBinding")
}

func TestKustomizePolicy_BindingsReferenceExistingPolicies(t *testing.T) {
	testCases := []struct {
		name string
		dir  string
	}{
		{
			name: "config-policy",
			dir:  filepath.Join("..", "..", "config", "policy"),
		},
		{
			name: "config-default",
			dir:  filepath.Join("..", "..", "config", "default"),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			yamlBytes := kustomizeBuild(t, tc.dir)
			objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
				gvk := u.GroupVersionKind()
				return gvk.Group == "admissionregistration.k8s.io" &&
					(gvk.Kind == "ValidatingAdmissionPolicy" || gvk.Kind == "ValidatingAdmissionPolicyBinding")
			})

			policies := make(map[string]struct{})
			bindings := make(map[string]string)
			for _, obj := range objs {
				switch obj.GetKind() {
				case "ValidatingAdmissionPolicy":
					policies[obj.GetName()] = struct{}{}
				case "ValidatingAdmissionPolicyBinding":
					policyName, found, err := unstructured.NestedString(obj.Object, "spec", "policyName")
					if err != nil {
						t.Fatalf("read spec.policyName for binding %s: %v", obj.GetName(), err)
					}
					if !found || policyName == "" {
						t.Fatalf("binding %s has empty spec.policyName", obj.GetName())
					}
					bindings[obj.GetName()] = policyName
				}
			}

			if len(bindings) == 0 {
				t.Fatal("expected at least one ValidatingAdmissionPolicyBinding")
			}

			for bindingName, policyName := range bindings {
				if _, ok := policies[policyName]; !ok {
					t.Fatalf("binding %s references missing policy %s", bindingName, policyName)
				}
			}
		})
	}
}

func TestKustomizeDefault_LockManagedPolicyRequiresOpenBaoLabels(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
	objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
		gvk := u.GroupVersionKind()
		return gvk.Group == "admissionregistration.k8s.io" &&
			gvk.Kind == "ValidatingAdmissionPolicy" &&
			strings.HasSuffix(u.GetName(), "openbao-lock-managed-resource-mutations")
	})

	if len(objs) != 1 {
		t.Fatalf("expected exactly one openbao-lock-managed-resource-mutations policy, got %d", len(objs))
	}

	variables, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "variables")
	if err != nil {
		t.Fatalf("read spec.variables: %v", err)
	}
	if !found {
		t.Fatal("openbao-lock-managed-resource-mutations policy missing spec.variables")
	}

	var hasOpenBaoLabelExpression string
	var isManagedExpression string
	for _, variable := range variables {
		variableMap, ok := variable.(map[string]any)
		if !ok {
			continue
		}
		name, _ := variableMap["name"].(string)
		expression, _ := variableMap["expression"].(string)
		switch name {
		case "has_openbao_specific_label":
			hasOpenBaoLabelExpression = expression
		case "is_managed":
			isManagedExpression = expression
		}
	}

	if !strings.Contains(hasOpenBaoLabelExpression, `k.startsWith("openbao.org/")`) {
		t.Fatalf("has_openbao_specific_label expression does not enforce openbao.org/* label gate: %q", hasOpenBaoLabelExpression)
	}
	if !strings.Contains(isManagedExpression, "variables.has_openbao_specific_label") {
		t.Fatalf("is_managed expression does not require has_openbao_specific_label: %q", isManagedExpression)
	}
}

func TestKustomizeSingleTenantOverlay_BakesInNamespaceScopeAndRemovesProvisioner(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "overlays", "single-tenant"))
	objs := parseYAMLToUnstructured(t, yamlBytes, nil)

	var controller *unstructured.Unstructured
	var singleTenantBinding *unstructured.Unstructured
	var hasOperatorNamespace bool

	for _, obj := range objs {
		switch obj.GetKind() {
		case "Namespace":
			if obj.GetName() == "openbao-operator-system" {
				hasOperatorNamespace = true
			}
		case "Deployment":
			if obj.GetName() == "openbao-operator-controller" {
				controller = obj
			}
			if labels := obj.GetLabels(); labels["app.kubernetes.io/component"] == "provisioner" {
				t.Fatalf("unexpected provisioner deployment in single-tenant overlay: %s", obj.GetName())
			}
		case "Service":
			if labels := obj.GetLabels(); labels["app.kubernetes.io/component"] == "provisioner" {
				t.Fatalf("unexpected provisioner service in single-tenant overlay: %s", obj.GetName())
			}
		case "ServiceAccount":
			if labels := obj.GetLabels(); labels["app.kubernetes.io/component"] == "provisioner" {
				t.Fatalf("unexpected provisioner serviceaccount in single-tenant overlay: %s", obj.GetName())
			}
		case "ClusterRole":
			if labels := obj.GetLabels(); labels["app.kubernetes.io/component"] == "provisioner" {
				t.Fatalf("unexpected provisioner clusterrole in single-tenant overlay: %s", obj.GetName())
			}
		case "ClusterRoleBinding":
			if labels := obj.GetLabels(); labels["app.kubernetes.io/component"] == "provisioner" {
				t.Fatalf("unexpected provisioner clusterrolebinding in single-tenant overlay: %s", obj.GetName())
			}
		case "RoleBinding":
			if labels := obj.GetLabels(); labels["app.kubernetes.io/component"] == "provisioner" {
				t.Fatalf("unexpected provisioner rolebinding in single-tenant overlay: %s", obj.GetName())
			}
			if obj.GetName() == "openbao-operator-single-tenant" {
				singleTenantBinding = obj
			}
		}
	}

	if !hasOperatorNamespace {
		t.Fatal("single-tenant overlay did not include operator namespace")
	}
	if controller == nil {
		t.Fatal("single-tenant overlay missing controller deployment")
	}
	if singleTenantBinding == nil {
		t.Fatal("single-tenant overlay missing target namespace rolebinding")
	}
	if singleTenantBinding.GetNamespace() != "openbao" {
		t.Fatalf("single-tenant rolebinding namespace = %q, want %q", singleTenantBinding.GetNamespace(), "openbao")
	}

	envs, found, err := unstructured.NestedSlice(controller.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(envs) == 0 {
		t.Fatalf("read controller containers: found=%v err=%v", found, err)
	}
	container, ok := envs[0].(map[string]any)
	if !ok {
		t.Fatalf("controller container has unexpected type %T", envs[0])
	}
	envList, found, err := unstructured.NestedSlice(container, "env")
	if err != nil || !found {
		t.Fatalf("read controller env: found=%v err=%v", found, err)
	}
	var watchNamespace string
	for _, item := range envList {
		envMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := envMap["name"].(string); name == "WATCH_NAMESPACE" {
			watchNamespace, _ = envMap["value"].(string)
			break
		}
	}
	if watchNamespace != "openbao" {
		t.Fatalf("WATCH_NAMESPACE = %q, want %q", watchNamespace, "openbao")
	}

	subjects, found, err := unstructured.NestedSlice(singleTenantBinding.Object, "subjects")
	if err != nil || !found || len(subjects) != 1 {
		t.Fatalf("read rolebinding subjects: found=%v len=%d err=%v", found, len(subjects), err)
	}
	subject, ok := subjects[0].(map[string]any)
	if !ok {
		t.Fatalf("rolebinding subject has unexpected type %T", subjects[0])
	}
	if got, _ := subject["name"].(string); got != "openbao-operator-controller" {
		t.Fatalf("rolebinding subject name = %q, want %q", got, "openbao-operator-controller")
	}
	if got, _ := subject["namespace"].(string); got != "openbao-operator-system" {
		t.Fatalf("rolebinding subject namespace = %q, want %q", got, "openbao-operator-system")
	}
}

func isClusterScopedManifestObject(gvk schema.GroupVersionKind) bool {
	if gvk.Group == "rbac.authorization.k8s.io" && (gvk.Kind == "ClusterRole" || gvk.Kind == "ClusterRoleBinding") {
		return true
	}
	if gvk.Group == "admissionregistration.k8s.io" && (gvk.Kind == "ValidatingAdmissionPolicy" || gvk.Kind == "ValidatingAdmissionPolicyBinding") {
		return true
	}
	if gvk.Group == "apiextensions.k8s.io" && gvk.Kind == "CustomResourceDefinition" {
		return true
	}
	return false
}
