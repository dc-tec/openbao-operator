package workload

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestStatefulSet_WithAuditFileStorageMount(t *testing.T) {
	cluster := newMinimalCluster("audit-cluster", "default")
	cluster.Spec.AuditFileStorage = &openbaov1alpha1.AuditFileStorageConfig{
		Mode: openbaov1alpha1.AuditFileStorageModeManagedPVC,
		Size: "5Gi",
	}

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	volume, ok := getVolume(statefulSet.Spec.Template.Spec.Volumes, auditFileStorageVolumeName)
	if !ok {
		t.Fatal("expected StatefulSet to include audit file storage volume")
	}
	if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName != "audit-cluster-audit" {
		t.Fatalf("audit file storage volume = %#v, want PVC audit-cluster-audit", volume)
	}

	mount, ok := getVolumeMount(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, auditFileStorageVolumeName)
	if !ok {
		t.Fatal("expected OpenBao container to mount audit file storage volume")
	}
	if got, want := mount.MountPath, portopenbao.AuditFileStorageDefaultMountPath; got != want {
		t.Fatalf("audit file storage mount path = %q, want %q", got, want)
	}
	if got, want := mount.SubPathExpr, portopenbao.AuditFileStoragePodSubPathExpr; got != want {
		t.Fatalf("audit file storage subPathExpr = %q, want %q", got, want)
	}
}

func TestStatefulSet_WithAuditFileStorageCustomMountPath(t *testing.T) {
	cluster := newMinimalCluster("audit-cluster", "default")
	cluster.Spec.AuditFileStorage = &openbaov1alpha1.AuditFileStorageConfig{
		Mode:              openbaov1alpha1.AuditFileStorageModeExistingPVC,
		ExistingClaimName: "audit-rwx",
		MountPath:         "/var/log/openbao/audit",
	}

	mounts := buildContainerVolumeMounts(cluster, "/etc/bao/rendered-config")
	mount, ok := getVolumeMount(mounts, auditFileStorageVolumeName)
	if !ok {
		t.Fatal("expected OpenBao container to mount audit file storage volume")
	}
	if got, want := mount.MountPath, "/var/log/openbao/audit"; got != want {
		t.Fatalf("audit file storage mount path = %q, want %q", got, want)
	}
}

func TestStatefulSet_DoesNotMountAuditFileStorageWithoutResolvedClaim(t *testing.T) {
	cluster := newMinimalCluster("audit-cluster", "default")
	cluster.Spec.AuditFileStorage = &openbaov1alpha1.AuditFileStorageConfig{
		Mode:      openbaov1alpha1.AuditFileStorageModeExistingPVC,
		MountPath: "/var/log/openbao/audit",
	}

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	if _, ok := getVolume(statefulSet.Spec.Template.Spec.Volumes, auditFileStorageVolumeName); ok {
		t.Fatal("did not expect audit file storage volume without a resolved claim name")
	}
	if _, ok := getVolumeMount(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, auditFileStorageVolumeName); ok {
		t.Fatal("did not expect audit file storage mount without a resolved claim name")
	}
}

func TestStatefulSetAuditFileStorageRequiresRecreate(t *testing.T) {
	cluster := newMinimalCluster("audit-cluster", "default")
	cluster.Spec.AuditFileStorage = &openbaov1alpha1.AuditFileStorageConfig{
		Mode: openbaov1alpha1.AuditFileStorageModeManagedPVC,
		Size: "5Gi",
	}

	desired, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}
	existing := desired.DeepCopy()
	existing.Spec.Template.Spec.Volumes = removeVolume(existing.Spec.Template.Spec.Volumes, auditFileStorageVolumeName)
	for i := range existing.Spec.Template.Spec.Containers {
		existing.Spec.Template.Spec.Containers[i].VolumeMounts = removeVolumeMount(existing.Spec.Template.Spec.Containers[i].VolumeMounts, auditFileStorageVolumeName)
	}

	if !statefulSetAuditFileStorageRequiresRecreate(desired, existing) {
		t.Fatal("expected missing existing audit file storage volume/mount to require StatefulSet recreation")
	}
	if statefulSetAuditFileStorageRequiresRecreate(desired, desired.DeepCopy()) {
		t.Fatal("did not expect matching audit file storage volume/mount to require StatefulSet recreation")
	}
}

func removeVolume(volumes []corev1.Volume, name string) []corev1.Volume {
	filtered := volumes[:0]
	for _, volume := range volumes {
		if volume.Name != name {
			filtered = append(filtered, volume)
		}
	}
	return filtered
}

func removeVolumeMount(mounts []corev1.VolumeMount, name string) []corev1.VolumeMount {
	filtered := mounts[:0]
	for _, mount := range mounts {
		if mount.Name != name {
			filtered = append(filtered, mount)
		}
	}
	return filtered
}
