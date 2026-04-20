package statusops

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// StatusState holds observed cluster state used for status computation.
type StatusState struct {
	// StatefulSet observed state.
	StatefulSet   *appsv1.StatefulSet
	ReadyReplicas int32
	Available     bool
	StatusStale   bool // StatefulSet status may lag behind reality.

	// Read-replica StatefulSet observed state.
	ReadReplicaStatefulSet        *appsv1.StatefulSet
	ReadReplicaReadyReplicas      int32
	ReadReplicaRegisteredReplicas int32
	ReadReplicaHealthyReplicas    int32
	ReadReplicaMembershipKnown    bool
	ReadReplicaAutopilotKnown     bool
	ReadServingAvailable          bool
	ReadServingKnown              bool

	// Data PVC storage state.
	DataPVCCount             int
	DataPVCStorageClassNames []string
	DataPVCStorageClassUnset bool

	// Read-replica PVC storage state.
	ReadReplicaDataPVCCount             int
	ReadReplicaDataPVCStorageClassNames []string
	ReadReplicaDataPVCStorageClassUnset bool

	// Pod state.
	Pods             []corev1.Pod
	Pod0             *corev1.Pod
	LeaderName       string
	LeaderCount      int
	Initialized      bool
	InitializedKnown bool
	Sealed           bool
	SealedKnown      bool

	// Backup state.
	BackupJobName    string
	BackupInProgress bool

	// Upgrade state (computed from cluster.Status).
	UpgradeFailed            bool
	UpgradeInProgress        bool
	RollingUpgradeInProgress bool
	BlueGreenInProgress      bool

	// Active revision for blue/green deployments.
	ActiveRevision string
}
