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

func TestKustomizeDefault_ClusterScopedResourcesHaveNoNamespace(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
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

		if obj.GetNamespace() != "" {
			t.Fatalf("cluster-scoped %s %s has unexpected namespace %q", obj.GetKind(), obj.GetName(), obj.GetNamespace())
		}
	}
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
			strings.HasSuffix(u.GetName(), "lock-managed-resource-mutations")
	})

	if len(objs) != 1 {
		t.Fatalf("expected exactly one lock-managed-resource-mutations policy, got %d", len(objs))
	}

	variables, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "variables")
	if err != nil {
		t.Fatalf("read spec.variables: %v", err)
	}
	if !found {
		t.Fatal("lock-managed-resource-mutations policy missing spec.variables")
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
