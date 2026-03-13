//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVAP_ProvisionerTenantGovernance_DeniesWrongQuotaName(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	ensureProvisionerRBACApplied(t)

	namespace := newTestNamespace(t)
	provisionerClient := newImpersonatedClient(t, provisionerUsername)

	for attempt := 0; attempt < 25; attempt++ {
		quota := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unexpected-tenant-quota",
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "openbao-operator",
					"app.kubernetes.io/component":  "provisioner",
					"app.kubernetes.io/managed-by": "openbao-operator",
				},
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourcePods: resource.MustParse("10"),
				},
			},
		}

		err := provisionerClient.Create(ctx, quota)
		if err == nil {
			_ = k8sClient.Delete(ctx, quota)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "can only manage the operator tenant ResourceQuota and LimitRange guardrail objects") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny Provisioner quota creation with unexpected name after retries")
}

func TestVAP_ProvisionerTenantGovernance_DeniesMissingLabels(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	ensureProvisionerRBACApplied(t)

	namespace := newTestNamespace(t)
	provisionerClient := newImpersonatedClient(t, provisionerUsername)

	for attempt := 0; attempt < 25; attempt++ {
		limitRange := &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openbao-operator-tenant-limits",
				Namespace: namespace,
			},
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type: corev1.LimitTypeContainer,
						DefaultRequest: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("100m"),
						},
					},
				},
			},
		}

		err := provisionerClient.Create(ctx, limitRange)
		if err == nil {
			_ = k8sClient.Delete(ctx, limitRange)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "must label tenant ResourceQuota and LimitRange guardrails as operator-managed") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny Provisioner tenant guardrail creation without labels after retries")
}

func TestVAP_ProvisionerTenantGovernance_DeniesDirectGuardrailMutation(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	ensureProvisionerRBACApplied(t)

	namespace := newTestNamespace(t)
	provisionerClient := newImpersonatedClient(t, provisionerUsername)

	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbao-operator-tenant-quota",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openbao-operator",
				"app.kubernetes.io/component":  "provisioner",
				"app.kubernetes.io/managed-by": "openbao-operator",
			},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("10"),
			},
		},
	}
	if err := provisionerClient.Create(ctx, quota); err != nil {
		t.Fatalf("create provisioner-managed ResourceQuota: %v", err)
	}

	for attempt := 0; attempt < 25; attempt++ {
		var latest corev1.ResourceQuota
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: quota.Name}, &latest); err != nil {
			t.Fatalf("get ResourceQuota: %v", err)
		}

		original := latest.DeepCopy()
		latest.Spec.Hard[corev1.ResourcePods] = resource.MustParse("99")
		err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "Direct modification of the operator-managed tenant ResourceQuota and LimitRange guardrails is prohibited") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny direct mutation of provisioner-managed tenant guardrails after retries")
}
