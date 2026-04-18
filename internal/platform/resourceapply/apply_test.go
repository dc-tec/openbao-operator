package resourceapply

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestApplyOwnedSetsOwnerReference(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithReturnManagedFields().Build()

	owner := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", UID: "12345"}}
	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"}}

	if err := ApplyOwned(context.Background(), k8sClient, scheme, owner, obj); err != nil {
		t.Fatalf("ApplyOwned() error = %v", err)
	}

	stored := &corev1.ConfigMap{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), stored); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(stored.OwnerReferences) != 1 || stored.OwnerReferences[0].Name != owner.Name {
		t.Fatalf("expected controller owner reference for %q, got %#v", owner.Name, stored.OwnerReferences)
	}
}

func TestApplyUnownedDoesNotSetOwnerReference(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithReturnManagedFields().Build()

	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"}}

	if err := ApplyUnowned(context.Background(), k8sClient, obj); err != nil {
		t.Fatalf("ApplyUnowned() error = %v", err)
	}

	stored := &corev1.ConfigMap{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), stored); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(stored.OwnerReferences) != 0 {
		t.Fatalf("expected no owner references, got %#v", stored.OwnerReferences)
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1): %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(openbaov1alpha1): %v", err)
	}
	return scheme
}
