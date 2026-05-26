package openbao

import (
	"fmt"
	"path"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	// AuditFileStorageDefaultMountPath is the default shared filesystem mount for file audit logs.
	AuditFileStorageDefaultMountPath = "/openbao/audit"

	// AuditFileStoragePodSubPathExpr isolates each Pod under the shared PVC while preserving one rendered file path.
	AuditFileStoragePodSubPathExpr = "$(BAO_K8S_POD_NAME)"
)

// HasAuditFileStorage reports whether the cluster config includes shared audit file storage.
func HasAuditFileStorage(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil && cluster.Spec.AuditFileStorage != nil
}

// UsesManagedAuditFileStorage reports whether the operator should create a managed RWX PVC.
func UsesManagedAuditFileStorage(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return HasAuditFileStorage(cluster) &&
		cluster.Spec.AuditFileStorage.Mode == openbaov1alpha1.AuditFileStorageModeManagedPVC
}

// UsesExistingAuditFileStorage reports whether the operator should mount a pre-existing RWX PVC.
func UsesExistingAuditFileStorage(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return HasAuditFileStorage(cluster) &&
		cluster.Spec.AuditFileStorage.Mode == openbaov1alpha1.AuditFileStorageModeExistingPVC
}

// ManagedAuditFileStoragePVCName returns the operator-managed PVC name for the cluster.
func ManagedAuditFileStoragePVCName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil {
		return ""
	}
	return fmt.Sprintf("%s-audit", strings.TrimSpace(cluster.Name))
}

// AuditFileStorageClaimName resolves the PVC claim name when audit file storage is configured.
func AuditFileStorageClaimName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if !HasAuditFileStorage(cluster) {
		return ""
	}
	if UsesExistingAuditFileStorage(cluster) {
		return strings.TrimSpace(cluster.Spec.AuditFileStorage.ExistingClaimName)
	}
	return ManagedAuditFileStoragePVCName(cluster)
}

// AuditFileStorageMountPath returns the effective Pod mount path for audit file storage.
func AuditFileStorageMountPath(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if !HasAuditFileStorage(cluster) {
		return ""
	}
	mountPath := strings.TrimSpace(cluster.Spec.AuditFileStorage.MountPath)
	if mountPath == "" {
		return AuditFileStorageDefaultMountPath
	}
	return path.Clean(mountPath)
}

// PathUnderAuditFileStorage reports whether filePath is under the effective audit storage mount path.
func PathUnderAuditFileStorage(cluster *openbaov1alpha1.OpenBaoCluster, filePath string) bool {
	mountPath := AuditFileStorageMountPath(cluster)
	if mountPath == "" {
		return false
	}

	cleanMount := path.Clean(mountPath)
	cleanFile := path.Clean(strings.TrimSpace(filePath))
	return strings.HasPrefix(cleanFile, cleanMount+"/")
}
