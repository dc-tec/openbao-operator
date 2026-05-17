//go:build integration
// +build integration

package openshift

import (
	"os/exec"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func kustomizeBuild(t *testing.T, root string, rel string) []byte {
	t.Helper()

	kustomize := filepath.Join(root, "bin", "kustomize")
	if _, err := exec.LookPath(kustomize); err != nil {
		if p, pathErr := exec.LookPath("kustomize"); pathErr == nil {
			kustomize = p
		} else {
			t.Fatalf("kustomize binary not found at %q and not in PATH", kustomize)
		}
	}

	dir := filepath.Join(root, rel)
	cmd := exec.Command(kustomize, "build", dir) // #nosec G204 -- test invokes repo-managed kustomize with fixed args.
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kustomize build %q failed: %v\n%s", dir, err, string(out))
	}
	return out
}

func TestRenderedInstallerOperatorDeployments_AreOpenShiftAdmittable(t *testing.T) {
	root := repoRoot(t)
	objs := parseYAMLBytes(t, "kustomize build config/default", kustomizeBuild(t, root, "config/default"))

	want := map[string]bool{
		"openbao-operator-controller":  false,
		"openbao-operator-provisioner": false,
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
			t.Fatalf("rendered installer: expected Deployment %q not found", name)
		}
	}
}
