//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
	"time"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVAP_LockManagedPDB_DeniesDirectMutation(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	controllerClient := newPrivilegedImpersonatedClient(t, controllerUsername)

	maxUnavailable := intstr.FromInt(1)
	pdb := &policyv1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy/v1",
			Kind:       "PodDisruptionBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-pdb",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openbao",
				"app.kubernetes.io/instance":   "example",
				"app.kubernetes.io/managed-by": "openbao-operator",
				"openbao.org/cluster":          "example",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"openbao.org/cluster": "example",
				},
			},
		},
	}
	if err := controllerClient.Create(ctx, pdb); err != nil {
		t.Fatalf("create managed PodDisruptionBudget: %v", err)
	}

	for attempt := 0; attempt < 25; attempt++ {
		var latest policyv1.PodDisruptionBudget
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: pdb.Name}, &latest); err != nil {
			t.Fatalf("get managed PodDisruptionBudget: %v", err)
		}

		original := latest.DeepCopy()
		two := intstr.FromInt(2)
		latest.Spec.MaxUnavailable = &two
		err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "Direct modification of OpenBao-managed resources is prohibited") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny direct mutation of controller-managed PodDisruptionBudget after retries")
}
