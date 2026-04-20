package workload

// StatefulSetSpec encapsulates all parameters needed for StatefulSet reconciliation.
// This struct decouples the infrastructure layer from upgrade strategy knowledge.
type StatefulSetSpec struct {
	// Name is the StatefulSet name (e.g., "cluster-name" or "cluster-name-revision")
	Name string

	// Pool identifies the workload pool (for example, voter or read-replica).
	Pool string

	// Revision is the revision identifier (empty for non-revisioned StatefulSets)
	Revision string

	// Image is the container image to use (verified digest if available)
	Image string

	// InitContainerImage is the resolved init container image to use.
	// When operator image verification is enabled, this should be a digest.
	InitContainerImage string

	// Replicas is the desired replica count
	Replicas int32

	// ConfigHash is used for pod annotations to trigger restarts on config changes
	ConfigHash string

	// RestartAt overrides the effective pod-template restart annotation for this
	// workload pool. Nil falls back to the cluster-level runtime or deprecated
	// maintenance restart request.
	RestartAt *string

	// DisableSelfInit prevents pod self-initialization (used for Green pods in BlueGreen)
	DisableSelfInit bool

	// SkipReconciliation indicates the StatefulSet should not be reconciled
	// (e.g., during BlueGreen cleanup phases)
	SkipReconciliation bool
}
