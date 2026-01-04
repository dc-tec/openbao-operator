//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestVAP_SentinelMayOnlyUpdateSentinelTriggerFields(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)

	// Allow the sentinel identity to patch OpenBaoCluster status in this namespace (RBAC).
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sentinel-status-patch",
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters"},
				Verbs:     []string{"get"},
			},
			{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters/status"},
				Verbs:     []string{"patch"},
			},
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("create Role: %v", err)
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sentinel-status-patch",
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "openbao-sentinel",
				Namespace: namespace,
			},
		},
	}
	if err := k8sClient.Create(ctx, rb); err != nil {
		t.Fatalf("create RoleBinding: %v", err)
	}

	cluster := newMinimalClusterObj(namespace, "sentinel-vap")
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	sentinelUser := "system:serviceaccount:" + namespace + ":openbao-sentinel"
	sentinelClient := newImpersonatedClient(t, sentinelUser)

	// Allowed: patch only status.sentinel trigger fields.
	var latest openbaov1alpha1.OpenBaoCluster
	if err := sentinelClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, &latest); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	original := latest.DeepCopy()
	latest.Status.Sentinel = &openbaov1alpha1.SentinelStatus{
		TriggerID:       "t1",
		TriggeredAt:     &metav1.Time{Time: time.Now()},
		TriggerResource: "ConfigMap/foo",
	}

	if err := sentinelClient.Status().Patch(ctx, &latest, client.MergeFrom(original)); err != nil {
		t.Fatalf("expected sentinel trigger patch to succeed, got: %v", err)
	}

	// Forbidden: patch status.phase (even if also setting sentinel trigger fields).
	if err := sentinelClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, &latest); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	original = latest.DeepCopy()
	latest.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
	latest.Status.Sentinel = &openbaov1alpha1.SentinelStatus{
		TriggerID:       "t2",
		TriggeredAt:     &metav1.Time{Time: time.Now()},
		TriggerResource: "StatefulSet/bar",
	}

	err := sentinelClient.Status().Patch(ctx, &latest, client.MergeFrom(original))
	if err == nil {
		t.Fatalf("expected sentinel status patch that changes non-sentinel fields to be denied")
	}
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "Sentinel may only update status.sentinel trigger fields") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
