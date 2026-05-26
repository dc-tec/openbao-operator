package bootstrap

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestBuildManagedAuditFileStoragePVC(t *testing.T) {
	storageClass := "rwx"
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			AuditFileStorage: &openbaov1alpha1.AuditFileStorageConfig{
				Mode:             openbaov1alpha1.AuditFileStorageModeManagedPVC,
				Size:             "5Gi",
				StorageClassName: &storageClass,
			},
		},
	}

	pvc, err := buildManagedAuditFileStoragePVC(cluster)
	if err != nil {
		t.Fatalf("buildManagedAuditFileStoragePVC() error = %v", err)
	}
	if got, want := pvc.Name, portopenbao.ManagedAuditFileStoragePVCName(cluster); got != want {
		t.Fatalf("PVC name = %q, want %q", got, want)
	}
	if got, want := pvc.Namespace, "default"; got != want {
		t.Fatalf("PVC namespace = %q, want %q", got, want)
	}
	if len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != "ReadWriteMany" {
		t.Fatalf("PVC access modes = %#v, want ReadWriteMany", pvc.Spec.AccessModes)
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != storageClass {
		t.Fatalf("PVC storageClassName = %#v, want %q", pvc.Spec.StorageClassName, storageClass)
	}
	if got := pvc.Labels[constants.LabelOpenBaoAuditFileStorage]; got != "true" {
		t.Fatalf("audit storage label = %q, want true", got)
	}
	if got := pvc.Labels[constants.LabelOpenBaoSensitive]; got != constants.LabelValueSensitiveAudit {
		t.Fatalf("sensitive label = %q, want %q", got, constants.LabelValueSensitiveAudit)
	}
}

func TestBuildManagedAuditFileStoragePVCRejectsInvalidSize(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			AuditFileStorage: &openbaov1alpha1.AuditFileStorageConfig{
				Mode: openbaov1alpha1.AuditFileStorageModeManagedPVC,
				Size: "not-a-size",
			},
		},
	}

	if _, err := buildManagedAuditFileStoragePVC(cluster); err == nil {
		t.Fatal("buildManagedAuditFileStoragePVC() expected error, got nil")
	}
}
