//go:build integration
// +build integration

package integration

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const integrationFieldOwner = "openbao-operator-integration-tests"

var (
	onceDefaultPolicies sync.Once
	onceProvisionerRBAC sync.Once
)

func ensureDefaultAdmissionPoliciesApplied(t *testing.T) {
	t.Helper()

	onceDefaultPolicies.Do(func() {
		yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))

		objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
			gvk := u.GroupVersionKind()
			if gvk.Group != testAdmissionRegistrationGroup {
				return false
			}
			switch gvk.Kind {
			case testKindVAP:
				return isDefaultIntegrationPolicyNameAllowed(u.GetName())
			case testKindVAPBinding:
				policyName, found, err := unstructured.NestedString(u.Object, "spec", "policyName")
				if err != nil || !found || policyName == "" {
					return false
				}
				return isDefaultIntegrationPolicyNameAllowed(policyName)
			default:
				return false
			}
		})

		applyUnstructuredObjects(t, objs)
	})
}

func isDefaultIntegrationPolicyNameAllowed(policyName string) bool {
	allowedSuffixes := []string{
		"openbao-validate-openbaocluster",
		"openbao-validate-openbaoclusterclaimbackuprequest",
		"openbao-validate-openbaoclusterclaimrestorerequest",
		"openbao-validate-openbaoclusterclaimupgraderequest",
		"openbao-validate-openbaorestore",
		"openbao-validate-openbao-tenant",
		"openbao-lock-managed-resource-mutations",
		"openbao-lock-materialized-openbaoclusterclaim-spec",
		"openbao-restrict-provisioner-rbac",
		"openbao-restrict-provisioner-tenant-governance",
		"openbao-restrict-controller-serviceaccounts",
		"openbao-restrict-controller-secret-writes",
	}
	for _, suffix := range allowedSuffixes {
		if strings.HasSuffix(policyName, suffix) {
			return true
		}
	}
	return false
}

func ensureProvisionerRBACApplied(t *testing.T) {
	t.Helper()

	onceProvisionerRBAC.Do(func() {
		yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))

		objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
			gvk := u.GroupVersionKind()
			if gvk.Group != testRBACGroup {
				return false
			}

			switch gvk.Kind {
			case testKindClusterRole:
				return strings.Contains(u.GetName(), "provisioner-role")
			case testKindClusterRoleBinding:
				return strings.Contains(u.GetName(), "provisioner-rolebinding")
			default:
				return false
			}
		})

		applyUnstructuredObjects(t, objs)
	})
}

func newImpersonatedClient(t *testing.T, username string) client.Client {
	t.Helper()

	return newImpersonatedClientWithGroups(t, username)
}

func newImpersonatedClientWithGroups(t *testing.T, username string, groups ...string) client.Client {
	t.Helper()

	impersonated := rest.CopyConfig(cfg)
	impersonated.Impersonate = rest.ImpersonationConfig{
		UserName: username,
		Groups:   groups,
	}

	c, err := client.New(impersonated, client.Options{Scheme: k8sScheme})
	if err != nil {
		t.Fatalf("create impersonated client: %v", err)
	}
	return c
}

func newControllerClient(t *testing.T) client.Client {
	t.Helper()

	return newPrivilegedImpersonatedClient(t, controllerUsername)
}

func kustomizeBuild(t *testing.T, dir string) []byte {
	t.Helper()

	kustomize := filepath.Join("..", "..", "bin", "kustomize")
	if _, err := exec.LookPath(kustomize); err != nil {
		// The repo typically provides a local bin/; if not, fall back to PATH.
		if p, pathErr := exec.LookPath("kustomize"); pathErr == nil {
			kustomize = p
		} else {
			t.Fatalf("kustomize binary not found at %q and not in PATH", kustomize)
		}
	}

	cmd := exec.Command(kustomize, "build", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kustomize build %q failed: %v\n%s", dir, err, string(out))
	}
	return out
}

func parseYAMLToUnstructured(
	t *testing.T,
	yamlBytes []byte,
	keep func(*unstructured.Unstructured) bool,
) []*unstructured.Unstructured {
	t.Helper()

	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), 4096)
	var out []*unstructured.Unstructured

	for {
		var raw map[string]any
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode YAML: %v", err)
		}

		if len(raw) == 0 {
			continue
		}

		u := &unstructured.Unstructured{Object: raw}
		if u.GetAPIVersion() == "" || u.GetKind() == "" || u.GetName() == "" {
			continue
		}

		// Normalize cluster-scoped resources: remove accidental namespaces from kustomize output.
		if isClusterScoped(u.GroupVersionKind()) {
			u.SetNamespace("")
		}

		if keep != nil && !keep(u) {
			continue
		}
		out = append(out, u)
	}

	return out
}

func isClusterScoped(gvk schema.GroupVersionKind) bool {
	if gvk.Group == testRBACGroup && (gvk.Kind == testKindClusterRole || gvk.Kind == testKindClusterRoleBinding) {
		return true
	}
	if gvk.Group == testAdmissionRegistrationGroup &&
		(gvk.Kind == testKindVAP || gvk.Kind == testKindVAPBinding) {
		return true
	}
	return false
}

func applyUnstructuredObjects(t *testing.T, objs []*unstructured.Unstructured) {
	t.Helper()

	for _, obj := range objs {
		obj.SetManagedFields(nil)
		if err := k8sClient.Apply(
			ctx,
			client.ApplyConfigurationFromUnstructured(obj),
			client.ForceOwnership,
			client.FieldOwner(integrationFieldOwner),
		); err != nil {
			t.Fatalf("apply %s %s/%s: %v", obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
		}
	}
}

func applyClientObject(t *testing.T, c client.Client, obj client.Object) {
	t.Helper()

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatalf("convert %s/%s to unstructured: %v", obj.GetNamespace(), obj.GetName(), err)
	}

	u := &unstructured.Unstructured{Object: unstructuredMap}
	u.SetManagedFields(nil)
	if err := c.Apply(
		ctx,
		client.ApplyConfigurationFromUnstructured(u),
		client.ForceOwnership,
		client.FieldOwner(integrationFieldOwner),
	); err != nil {
		t.Fatalf("apply %s %s/%s: %v", u.GetKind(), u.GetNamespace(), u.GetName(), err)
	}
}
