package backup

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestManualTriggerToken(t *testing.T) {
	t.Run("returns non-empty trigger", func(t *testing.T) {
		cluster := newTestClusterWithBackup("manual-backup", "backup-ns")
		cluster.Annotations = map[string]string{constants.AnnotationTriggerBackup: "now"}

		if got := manualTriggerToken(cluster); got != "now" {
			t.Fatalf("manualTriggerToken() = %q, want now", got)
		}
		if cluster.Annotations[constants.AnnotationTriggerBackup] != "now" {
			t.Fatal("manual trigger annotation was cleared unexpectedly")
		}
	})

	t.Run("ignores empty trigger annotation", func(t *testing.T) {
		cluster := newTestClusterWithBackup("manual-empty", "backup-ns")
		cluster.Annotations = map[string]string{constants.AnnotationTriggerBackup: ""}

		if got := manualTriggerToken(cluster); got != "" {
			t.Fatalf("manualTriggerToken() = %q, want empty", got)
		}
	})

	t.Run("returns empty trigger for nil cluster", func(t *testing.T) {
		if got := manualTriggerToken(nil); got != "" {
			t.Fatalf("manualTriggerToken() = %q, want empty", got)
		}
	})
}

func TestSkipManualTriggerForActiveJob(t *testing.T) {
	cluster := newTestClusterWithBackup("manual-active", "backup-ns")
	cluster.Annotations = map[string]string{
		constants.AnnotationTriggerBackup: "now",
		testKeepAnnotationKey:             testKeepAnnotationValue,
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		Build()

	manager := newBackupManager(k8sClient)
	if err := manager.skipManualTriggerForActiveJob(context.Background(), logr.Discard(), cluster, "now"); err != nil {
		t.Fatalf("skipManualTriggerForActiveJob() error = %v", err)
	}
	if _, ok := cluster.Annotations[constants.AnnotationTriggerBackup]; ok {
		t.Fatal("manual trigger annotation still present on in-memory object")
	}
	if got := cluster.Annotations[testKeepAnnotationKey]; got != testKeepAnnotationValue {
		t.Fatalf("unrelated annotation = %q, want %s", got, testKeepAnnotationValue)
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatalf("Get() cluster error = %v", err)
	}
	if _, ok := updated.Annotations[constants.AnnotationTriggerBackup]; ok {
		t.Fatal("manual trigger annotation still present on persisted object")
	}
	if got := updated.Annotations[testKeepAnnotationKey]; got != testKeepAnnotationValue {
		t.Fatalf("persisted unrelated annotation = %q, want %s", got, testKeepAnnotationValue)
	}
}

func TestClearTriggerAnnotationReturnsPatchError(t *testing.T) {
	cluster := newTestClusterWithBackup("manual-patch-error", "backup-ns")
	cluster.Annotations = map[string]string{
		constants.AnnotationTriggerBackup: "now",
		testKeepAnnotationKey:             testKeepAnnotationValue,
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return errors.New("patch failed")
			},
		}).
		Build()

	manager := newBackupManager(k8sClient)
	err := manager.clearManualTriggerAnnotation(context.Background(), logr.Discard(), cluster)
	if err == nil || err.Error() != "failed to clear manual backup trigger annotation: patch failed" {
		t.Fatalf("clearManualTriggerAnnotation() error = %v, want wrapped patch failure", err)
	}
	if _, ok := cluster.Annotations[constants.AnnotationTriggerBackup]; ok {
		t.Fatal("manual trigger annotation still present on in-memory object after patch failure")
	}
	if got := cluster.Annotations[testKeepAnnotationKey]; got != testKeepAnnotationValue {
		t.Fatalf("in-memory unrelated annotation = %q, want %s", got, testKeepAnnotationValue)
	}

	persisted := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, persisted); err != nil {
		t.Fatalf("Get() cluster error = %v", err)
	}
	if _, ok := persisted.Annotations[constants.AnnotationTriggerBackup]; !ok {
		t.Fatal("persisted manual trigger annotation removed despite patch failure")
	}
	if got := persisted.Annotations[testKeepAnnotationKey]; got != testKeepAnnotationValue {
		t.Fatalf("persisted unrelated annotation = %q, want %s", got, testKeepAnnotationValue)
	}
}
