package openshift

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
)

func repoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func parseYAMLFile(t *testing.T, path string) []*unstructured.Unstructured {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(raw), 4096)
	var out []*unstructured.Unstructured

	for {
		var obj map[string]any
		if err := decoder.Decode(&obj); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode %q: %v", path, err)
		}

		if len(obj) == 0 {
			continue
		}

		u := &unstructured.Unstructured{Object: obj}
		if u.GetAPIVersion() == "" || u.GetKind() == "" || u.GetName() == "" {
			continue
		}
		out = append(out, u)
	}

	return out
}

func assertNoPinnedIDs(t *testing.T, deployment *unstructured.Unstructured) {
	t.Helper()

	podSC, _, _ := unstructured.NestedMap(deployment.Object, "spec", "template", "spec", "securityContext")
	if podSC != nil {
		for _, field := range []string{"runAsUser", "runAsGroup", "fsGroup"} {
			if _, ok := podSC[field]; ok {
				t.Fatalf("%s/%s pins pod securityContext.%s", deployment.GetKind(), deployment.GetName(), field)
			}
		}
	}

	containers, _, _ := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
	for _, c := range containers {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		name, _ := cm["name"].(string)

		sc, _ := cm["securityContext"].(map[string]any)
		if sc == nil {
			continue
		}

		for _, field := range []string{"runAsUser", "runAsGroup"} {
			if _, ok := sc[field]; ok {
				t.Fatalf("%s/%s pins container %q securityContext.%s", deployment.GetKind(), deployment.GetName(), name, field)
			}
		}
	}
}

func TestManagerKustomizeManifests_AreOpenShiftAdmittable(t *testing.T) {
	root := repoRoot(t)

	for _, rel := range []string{
		filepath.Join("config", "manager", "controller.yaml"),
		filepath.Join("config", "manager", "provisioner.yaml"),
	} {
		path := filepath.Join(root, rel)
		objs := parseYAMLFile(t, path)

		var deployments []*unstructured.Unstructured
		for _, u := range objs {
			gvk := schema.FromAPIVersionAndKind(u.GetAPIVersion(), u.GetKind())
			if gvk.Group == "apps" && gvk.Kind == "Deployment" {
				deployments = append(deployments, u)
			}
		}

		if len(deployments) != 1 {
			t.Fatalf("%q: expected exactly 1 Deployment, got %d", rel, len(deployments))
		}

		assertNoPinnedIDs(t, deployments[0])
	}
}

func TestDistInstallOperatorDeployments_AreOpenShiftAdmittable(t *testing.T) {
	root := repoRoot(t)

	path := filepath.Join(root, "dist", "install.yaml")
	objs := parseYAMLFile(t, path)

	want := map[string]bool{
		"openbao-operator-openbao-operator-controller":  false,
		"openbao-operator-openbao-operator-provisioner": false,
	}

	for _, u := range objs {
		gvk := schema.FromAPIVersionAndKind(u.GetAPIVersion(), u.GetKind())
		if gvk.Group != "apps" || gvk.Kind != "Deployment" {
			continue
		}

		if _, ok := want[u.GetName()]; !ok {
			continue
		}

		assertNoPinnedIDs(t, u)
		want[u.GetName()] = true
	}

	for name, found := range want {
		if !found {
			t.Fatalf("dist/install.yaml: expected Deployment %q not found", name)
		}
	}
}
