//go:build integration
// +build integration

package integration

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func requireInvalidRequest(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected invalid request error, got nil")
	}

	if apierrors.IsInvalid(err) {
		return
	}

	var apiStatus apierrors.APIStatus
	if errors.As(err, &apiStatus) {
		status := apiStatus.Status()
		if status.Code == http.StatusUnprocessableEntity || status.Reason == metav1.StatusReasonInvalid {
			return
		}
	}

	if strings.Contains(err.Error(), "is invalid") {
		return
	}

	t.Fatalf("expected invalid request error, got %T: %v", err, err)
}

func TestCRD_OpenBaoRestore_RejectsMissingSpec(t *testing.T) {
	namespace := newTestNamespace(t)

	restore := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "openbao.org/v1alpha1",
			"kind":       "OpenBaoRestore",
			"metadata": map[string]any{
				"name":      "restore-missing-spec",
				"namespace": namespace,
			},
		},
	}

	err := k8sClient.Create(ctx, restore)
	requireInvalidRequest(t, err)
}

func TestCRD_OpenBaoCluster_RejectsMissingSpec(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "openbao.org/v1alpha1",
			"kind":       "OpenBaoCluster",
			"metadata": map[string]any{
				"name":      "cluster-missing-spec",
				"namespace": namespace,
			},
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireInvalidRequest(t, err)
}

func TestCRD_OpenBaoTenant_RejectsMissingSpec(t *testing.T) {
	namespace := newTestNamespace(t)

	tenant := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "openbao.org/v1alpha1",
			"kind":       "OpenBaoTenant",
			"metadata": map[string]any{
				"name":      "tenant-missing-spec",
				"namespace": namespace,
			},
		},
	}

	err := k8sClient.Create(ctx, tenant)
	requireInvalidRequest(t, err)
}

func TestVAP_OpenBaoRestore_RejectsSpecMutation(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	namespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		name := fmt.Sprintf("restore-immutable-%d", attempt)
		restore := &openbaov1alpha1.OpenBaoRestore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: openbaov1alpha1.OpenBaoRestoreSpec{
				Cluster: "cluster-1",
				Source: openbaov1alpha1.RestoreSource{
					Key: "backup.enc",
					Target: openbaov1alpha1.BackupTarget{
						Endpoint: "https://objectstore.example.com",
						Bucket:   "backups",
					},
				},
				JWTAuthRole: "restore-role",
			},
		}

		if err := k8sClient.Create(ctx, restore); err != nil {
			t.Fatalf("create OpenBaoRestore: %v", err)
		}

		var latest openbaov1alpha1.OpenBaoRestore
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restore.Name}, &latest); err != nil {
			t.Fatalf("get OpenBaoRestore: %v", err)
		}
		original := latest.DeepCopy()
		latest.Spec.Source.Key = "backup-v2.enc"

		err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
		if err == nil {
			_ = k8sClient.Delete(ctx, &latest)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "spec is immutable") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny OpenBaoRestore spec mutation after retries")
}

func TestVAP_OpenBaoCluster_RequiresProfile(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	namespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		cluster := newMinimalClusterObj(namespace, fmt.Sprintf("cluster-missing-profile-%d", attempt))
		cluster.Spec.Profile = ""

		err := k8sClient.Create(ctx, cluster)
		if err == nil {
			_ = k8sClient.Delete(ctx, cluster)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "spec.profile is required") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny OpenBaoCluster create without spec.profile after retries")
}

func TestVAP_OpenBaoCluster_AllowsDefaultInitContainer(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	namespace := newTestNamespace(t)

	// First ensure admission policies are active by waiting for a known denial.
	for attempt := 0; attempt < 25; attempt++ {
		invalid := newMinimalClusterObj(namespace, fmt.Sprintf("cluster-policy-probe-%d", attempt))
		invalid.Spec.Profile = ""

		err := k8sClient.Create(ctx, invalid)
		if err == nil {
			_ = k8sClient.Delete(ctx, invalid)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		requireAdmissionDenied(t, err)
		break
	}

	cluster := newMinimalClusterObj(namespace, "cluster-default-init")
	cluster.Spec.Profile = openbaov1alpha1.ProfileDevelopment
	cluster.Spec.InitContainer = nil

	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("expected OpenBaoCluster create without spec.initContainer to succeed, got: %v", err)
	}
}

func TestVAP_OpenBaoCluster_RejectsDisabledInitContainerOverride(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	namespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		cluster := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openbao.org/v1alpha1",
				"kind":       "OpenBaoCluster",
				"metadata": map[string]any{
					"name":      fmt.Sprintf("cluster-disabled-init-override-%d", attempt),
					"namespace": namespace,
				},
				"spec": map[string]any{
					"version":  "2.4.4",
					"image":    "openbao/openbao:2.4.4",
					"replicas": int64(3),
					"profile":  "Development",
					"tls": map[string]any{
						"enabled":        true,
						"rotationPeriod": "720h",
					},
					"storage": map[string]any{
						"size": "10Gi",
					},
					"initContainer": map[string]any{
						"enabled": false,
					},
				},
			},
		}

		err := k8sClient.Create(ctx, cluster)
		if err == nil {
			_ = k8sClient.Delete(ctx, cluster)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "spec.initContainer is optional; when set, spec.initContainer.enabled must be true.") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny OpenBaoCluster create with disabled initContainer override after retries")
}

func TestVAP_OpenBaoTenant_RejectsCrossNamespaceSelfService(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	targetNamespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		tenant := &openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("tenant-self-service-%d", attempt),
				Namespace: namespace,
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{
				TargetNamespace: targetNamespace,
			},
		}

		err := k8sClient.Create(ctx, tenant)
		if err == nil {
			_ = k8sClient.Delete(ctx, tenant)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "can only target its own namespace") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny cross-namespace OpenBaoTenant create after retries")
}

func TestVAP_OpenBaoTenant_RejectsTargetNamespaceMutation(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	operatorNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openbao-operator-system",
		},
	}
	if err := k8sClient.Create(ctx, operatorNamespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create operator namespace: %v", err)
	}

	targetNamespace := newTestNamespace(t)
	otherTargetNamespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		tenant := &openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("tenant-immutable-%d", attempt),
				Namespace: operatorNamespace.Name,
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{
				TargetNamespace: targetNamespace,
			},
		}

		if err := k8sClient.Create(ctx, tenant); err != nil {
			t.Fatalf("create OpenBaoTenant: %v", err)
		}

		var latest openbaov1alpha1.OpenBaoTenant
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: tenant.Namespace, Name: tenant.Name}, &latest); err != nil {
			t.Fatalf("get OpenBaoTenant: %v", err)
		}
		original := latest.DeepCopy()
		latest.Spec.TargetNamespace = otherTargetNamespace

		err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
		if err == nil {
			_ = k8sClient.Delete(ctx, &latest)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "spec.targetNamespace is immutable") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny OpenBaoTenant targetNamespace mutation after retries")
}
