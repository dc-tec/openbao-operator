package openbao

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestAuditFileStorageHelpers(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			AuditFileStorage: &openbaov1alpha1.AuditFileStorageConfig{
				Mode: openbaov1alpha1.AuditFileStorageModeManagedPVC,
				Size: "5Gi",
			},
		},
	}

	if !HasAuditFileStorage(cluster) {
		t.Fatal("expected audit file storage to be detected")
	}
	if !UsesManagedAuditFileStorage(cluster) {
		t.Fatal("expected managed audit file storage")
	}
	if got, want := AuditFileStorageClaimName(cluster), "example-audit"; got != want {
		t.Fatalf("AuditFileStorageClaimName() = %q, want %q", got, want)
	}
	if got, want := AuditFileStorageMountPath(cluster), AuditFileStorageDefaultMountPath; got != want {
		t.Fatalf("AuditFileStorageMountPath() = %q, want %q", got, want)
	}
	if !PathUnderAuditFileStorage(cluster, "/openbao/audit/example.jsonl") {
		t.Fatal("expected path under audit file storage")
	}
	if PathUnderAuditFileStorage(cluster, "/openbao/audit") {
		t.Fatal("did not expect mount directory itself to be treated as an audit file path")
	}
	if PathUnderAuditFileStorage(cluster, "/openbao/audit-archive/example.jsonl") {
		t.Fatal("did not expect sibling path to be treated as under audit file storage")
	}
}

func TestAuditFileStorageExistingClaim(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			AuditFileStorage: &openbaov1alpha1.AuditFileStorageConfig{
				Mode:              openbaov1alpha1.AuditFileStorageModeExistingPVC,
				ExistingClaimName: "audit-rwx",
				MountPath:         "/var/log/openbao/audit/../audit",
			},
		},
	}

	if !UsesExistingAuditFileStorage(cluster) {
		t.Fatal("expected existing audit file storage")
	}
	if got, want := AuditFileStorageClaimName(cluster), "audit-rwx"; got != want {
		t.Fatalf("AuditFileStorageClaimName() = %q, want %q", got, want)
	}
	if got, want := AuditFileStorageMountPath(cluster), "/var/log/openbao/audit"; got != want {
		t.Fatalf("AuditFileStorageMountPath() = %q, want %q", got, want)
	}
}
