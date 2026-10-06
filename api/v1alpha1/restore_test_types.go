package v1alpha1

// BackupSnapshotSummary is the latest successful executor's bounded observation.
// Identity and version are observed before and after streaming the snapshot.
type BackupSnapshotSummary struct {
	// Digest is sha256: followed by the lowercase digest of the uploaded snapshot.
	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	Digest string `json:"digest"`
	// ClusterID is the source native cluster ID observed during the backup.
	// +kubebuilder:validation:MaxLength=128
	ClusterID string `json:"clusterID"`
	// Version is the source OpenBao version observed during the backup.
	// +kubebuilder:validation:MaxLength=64
	Version string `json:"version"`
	// Size is the snapshot size in bytes.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=8589934592
	Size int64 `json:"size"`
}
