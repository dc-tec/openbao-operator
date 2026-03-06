package infra

// StatefulSetSpec encapsulates all parameters needed for StatefulSet reconciliation.
// This struct decouples the infrastructure layer from upgrade strategy knowledge.
type StatefulSetSpec struct {
	// Name is the StatefulSet name (e.g., "cluster-name" or "cluster-name-revision")
	Name string

	// Revision is the revision identifier (empty for non-revisioned StatefulSets)
	Revision string

	// Image is the container image to use (verified digest if available)
	Image string

	// InitContainerImage is the init container image (verified digest if available)
	InitContainerImage string

	// Replicas is the desired replica count
	Replicas int32

	// ConfigHash is used for pod annotations to trigger restarts on config changes
	ConfigHash string

	// DisableSelfInit prevents pod self-initialization (used for Green pods in BlueGreen)
	DisableSelfInit bool

	// SkipReconciliation indicates the StatefulSet should not be reconciled
	// (e.g., during BlueGreen cleanup phases)
	SkipReconciliation bool
}
