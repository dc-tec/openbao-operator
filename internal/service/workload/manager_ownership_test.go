package workload

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func TestDeleteStatefulSetIfExistsRejectsUnownedStatefulSet(t *testing.T) {
	cluster := workloadOwnershipCluster()
	statefulSet := workloadOwnershipStatefulSet(cluster.Name, cluster.Namespace, nil, 0)
	mgr := workloadOwnershipManager(cluster, statefulSet)

	err := mgr.DeleteStatefulSetIfExists(context.Background(), logr.Discard(), cluster, StatefulSetSpec{Name: statefulSet.Name})
	if err == nil || !strings.Contains(err.Error(), "requires OpenBaoCluster owner proof") {
		t.Fatalf("DeleteStatefulSetIfExists() error = %v, want owner proof error", err)
	}

	stored := &appsv1.StatefulSet{}
	if getErr := mgr.client.Get(context.Background(), client.ObjectKeyFromObject(statefulSet), stored); getErr != nil {
		t.Fatalf("expected unowned StatefulSet to remain: %v", getErr)
	}
}

func TestScaleStatefulSetIfExistsRejectsUnownedStatefulSet(t *testing.T) {
	cluster := workloadOwnershipCluster()
	statefulSet := workloadOwnershipStatefulSet(cluster.Name, cluster.Namespace, nil, 3)
	mgr := workloadOwnershipManager(cluster, statefulSet)

	err := mgr.ScaleStatefulSetIfExists(context.Background(), logr.Discard(), cluster, StatefulSetSpec{Name: statefulSet.Name}, 0)
	if err == nil || !strings.Contains(err.Error(), "requires OpenBaoCluster owner proof") {
		t.Fatalf("ScaleStatefulSetIfExists() error = %v, want owner proof error", err)
	}

	stored := &appsv1.StatefulSet{}
	if getErr := mgr.client.Get(context.Background(), client.ObjectKeyFromObject(statefulSet), stored); getErr != nil {
		t.Fatalf("expected unowned StatefulSet to remain: %v", getErr)
	}
	if stored.Spec.Replicas == nil || *stored.Spec.Replicas != 3 {
		t.Fatalf("stored StatefulSet replicas = %v, want 3", stored.Spec.Replicas)
	}
}

func TestEnsureStatefulSetRejectsUnownedExistingStatefulSet(t *testing.T) {
	cluster := workloadOwnershipCluster()
	statefulSet := workloadOwnershipStatefulSet(cluster.Name, cluster.Namespace, nil, 3)
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceidentity.TLSServerSecretName(cluster),
			Namespace: cluster.Namespace,
		},
	}
	mgr := workloadOwnershipManager(cluster, tlsSecret, statefulSet)

	err := mgr.EnsureStatefulSet(context.Background(), logr.Discard(), cluster, "listener \"tcp\" {}", StatefulSetSpec{Name: statefulSet.Name})
	if err == nil || !strings.Contains(err.Error(), "requires OpenBaoCluster owner proof") {
		t.Fatalf("EnsureStatefulSet() error = %v, want owner proof error", err)
	}

	stored := &appsv1.StatefulSet{}
	if getErr := mgr.client.Get(context.Background(), client.ObjectKeyFromObject(statefulSet), stored); getErr != nil {
		t.Fatalf("expected unowned StatefulSet to remain: %v", getErr)
	}
	if len(stored.OwnerReferences) != 0 {
		t.Fatalf("expected unowned StatefulSet owner references to remain empty, got %#v", stored.OwnerReferences)
	}
}

func TestStatefulSetMutationAllowsOwnedStatefulSet(t *testing.T) {
	cluster := workloadOwnershipCluster()
	statefulSet := workloadOwnershipStatefulSet(cluster.Name, cluster.Namespace, workloadOwnershipRef(cluster), 3)
	mgr := workloadOwnershipManager(cluster, statefulSet)

	if err := mgr.ScaleStatefulSetIfExists(context.Background(), logr.Discard(), cluster, StatefulSetSpec{Name: statefulSet.Name}, 0); err != nil {
		t.Fatalf("ScaleStatefulSetIfExists() error = %v", err)
	}
	stored := &appsv1.StatefulSet{}
	if err := mgr.client.Get(context.Background(), client.ObjectKeyFromObject(statefulSet), stored); err != nil {
		t.Fatalf("get scaled StatefulSet: %v", err)
	}
	if stored.Spec.Replicas == nil || *stored.Spec.Replicas != 0 {
		t.Fatalf("stored StatefulSet replicas = %v, want 0", stored.Spec.Replicas)
	}

	if err := mgr.DeleteStatefulSetIfExists(context.Background(), logr.Discard(), cluster, StatefulSetSpec{Name: statefulSet.Name}); err != nil {
		t.Fatalf("DeleteStatefulSetIfExists() error = %v", err)
	}
	if err := mgr.client.Get(context.Background(), client.ObjectKeyFromObject(statefulSet), &appsv1.StatefulSet{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected owned StatefulSet to be deleted, got error %v", err)
	}
}

func workloadOwnershipCluster() *openbaov1alpha1.OpenBaoCluster {
	cluster := newMinimalCluster("owned-workload", "default")
	cluster.UID = types.UID("owned-workload-uid")
	return cluster
}

func workloadOwnershipManager(cluster *openbaov1alpha1.OpenBaoCluster, objects ...client.Object) *Manager {
	builder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(cluster)
	if len(objects) > 0 {
		builder = builder.WithObjects(objects...)
	}
	c := builder.Build()
	return NewManager(c, testScheme, "")
}

func workloadOwnershipStatefulSet(name, namespace string, ownerRef *metav1.OwnerReference, replicas int32) *appsv1.StatefulSet {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(replicas),
		},
	}
	if ownerRef != nil {
		statefulSet.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}
	return statefulSet
}

func workloadOwnershipRef(cluster *openbaov1alpha1.OpenBaoCluster) *metav1.OwnerReference {
	controller := true
	return &metav1.OwnerReference{
		APIVersion: openbaov1alpha1.GroupVersion.String(),
		Kind:       "OpenBaoCluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
		Controller: &controller,
	}
}
