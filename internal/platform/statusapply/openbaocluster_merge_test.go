package statusapply

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const testMergedCurrentVersion = "2.5.0"

func TestPatchOpenBaoClusterStatusMerge_MutatesStatus(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "merge-cluster",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				TargetVersion: "2.5.0",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(newOpenBaoClusterStatusTestScheme(t)).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		Build()

	desired, err := PatchOpenBaoClusterStatusMerge(
		context.Background(),
		k8sClient,
		client.ObjectKeyFromObject(cluster),
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			obj.Status.Upgrade = nil
			obj.Status.CurrentVersion = testMergedCurrentVersion
			return nil
		},
	)
	if err != nil {
		t.Fatalf("PatchOpenBaoClusterStatusMerge() error = %v", err)
	}

	if desired.Status.Upgrade != nil {
		t.Fatalf("desired.Status.Upgrade = %#v, want nil", desired.Status.Upgrade)
	}
	if desired.Status.CurrentVersion != testMergedCurrentVersion {
		t.Fatalf("desired.Status.CurrentVersion = %q, want %s", desired.Status.CurrentVersion, testMergedCurrentVersion)
	}

	stored := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), stored); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.Status.Upgrade != nil {
		t.Fatalf("stored.Status.Upgrade = %#v, want nil", stored.Status.Upgrade)
	}
	if stored.Status.CurrentVersion != testMergedCurrentVersion {
		t.Fatalf("stored.Status.CurrentVersion = %q, want %s", stored.Status.CurrentVersion, testMergedCurrentVersion)
	}
}

func TestPatchOpenBaoClusterStatusMerge_PropagatesMutatorError(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "merge-error",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(newOpenBaoClusterStatusTestScheme(t)).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		Build()

	wantErr := errors.New("boom")
	_, err := PatchOpenBaoClusterStatusMerge(
		context.Background(),
		k8sClient,
		types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			return wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("PatchOpenBaoClusterStatusMerge() error = %v, want %v", err, wantErr)
	}
}

func TestFinalizeRootUpgradeStatusMerge(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "finalize-cluster",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				TargetVersion: "2.5.0",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(newOpenBaoClusterStatusTestScheme(t)).
		WithStatusSubresource(cluster).
		WithObjects(cluster.DeepCopy()).
		Build()

	desired, err := FinalizeRootUpgradeStatusMerge(
		context.Background(),
		k8sClient,
		client.ObjectKeyFromObject(cluster),
		testMergedCurrentVersion,
	)
	if err != nil {
		t.Fatalf("FinalizeRootUpgradeStatusMerge() error = %v", err)
	}
	if desired.Status.Upgrade != nil {
		t.Fatalf("desired.Status.Upgrade = %#v, want nil", desired.Status.Upgrade)
	}
	if desired.Status.CurrentVersion != testMergedCurrentVersion {
		t.Fatalf("desired.Status.CurrentVersion = %q, want %s", desired.Status.CurrentVersion, testMergedCurrentVersion)
	}
}
