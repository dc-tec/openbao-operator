//go:build integration
// +build integration

package integration

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
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
