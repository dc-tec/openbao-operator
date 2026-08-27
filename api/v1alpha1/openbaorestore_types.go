/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// RestorePhase represents the current phase of a restore operation.
// +kubebuilder:validation:Enum=Pending;Validating;Running;Completed;Failed;Unknown
type RestorePhase string

const (
	// OpenBaoRestoreFinalizer is the finalizer used to ensure lock cleanup logic
	// runs before an OpenBaoRestore is fully deleted.
	OpenBaoRestoreFinalizer = "openbao.org/openbaorestore-finalizer"

	// RestorePhasePending indicates the restore has been created but not yet started.
	RestorePhasePending RestorePhase = "Pending"
	// RestorePhaseValidating indicates the controller is validating preconditions.
	RestorePhaseValidating RestorePhase = "Validating"
	// RestorePhaseRunning indicates the restore job is executing.
	RestorePhaseRunning RestorePhase = "Running"
	// RestorePhaseCompleted indicates the restore completed successfully.
	RestorePhaseCompleted RestorePhase = "Completed"
	// RestorePhaseFailed indicates the restore failed.
	RestorePhaseFailed RestorePhase = "Failed"
	// RestorePhaseUnknown indicates the controller cannot determine whether the
	// destructive restore operation ran. The controller does not retry an
	// execution in this phase.
	RestorePhaseUnknown RestorePhase = "Unknown"
)

// RestoreExecutionStage identifies the durable execution boundary reached by a restore.
// +kubebuilder:validation:Enum=Prepared;Committed;Created;TerminalObserved;FollowThroughComplete;Unknown
type RestoreExecutionStage string

const (
	// RestoreExecutionStagePrepared indicates validation and resource preparation
	// completed, but Job creation has not been committed.
	RestoreExecutionStagePrepared RestoreExecutionStage = "Prepared"
	// RestoreExecutionStageCommitted indicates the controller durably committed to
	// one Job creation attempt. A missing Job after this point is ambiguous and is
	// not recreated automatically.
	RestoreExecutionStageCommitted RestoreExecutionStage = "Committed"
	// RestoreExecutionStageCreated indicates the controller persisted the created Job identity.
	RestoreExecutionStageCreated RestoreExecutionStage = "Created"
	// RestoreExecutionStageTerminalObserved indicates the controller persisted the terminal Job result.
	RestoreExecutionStageTerminalObserved RestoreExecutionStage = "TerminalObserved"
	// RestoreExecutionStageFollowThroughComplete indicates post-restore voter and
	// read-replica recovery completed.
	RestoreExecutionStageFollowThroughComplete RestoreExecutionStage = "FollowThroughComplete"
	// RestoreExecutionStageUnknown indicates the controller cannot prove whether
	// the committed execution ran.
	RestoreExecutionStageUnknown RestoreExecutionStage = "Unknown"
)

// RestoreExecutionResult is the persisted terminal result of a restore Job.
// +kubebuilder:validation:Enum=Succeeded;Failed
type RestoreExecutionResult string

const (
	// RestoreExecutionResultSucceeded indicates the restore Job succeeded.
	RestoreExecutionResultSucceeded RestoreExecutionResult = "Succeeded"
	// RestoreExecutionResultFailed indicates the restore Job failed.
	RestoreExecutionResultFailed RestoreExecutionResult = "Failed"
)

// RestoreExecutionStatus records the identity and durable receipts for one restore execution.
type RestoreExecutionStatus struct {
	// OperationID identifies this immutable restore execution.
	OperationID string `json:"operationID"`

	// Stage is the latest durable execution boundary observed by the controller.
	Stage RestoreExecutionStage `json:"stage"`

	// JobName is the expected restore Job name for this execution.
	JobName string `json:"jobName"`

	// JobUID is the UID returned for the created restore Job.
	// +optional
	JobUID types.UID `json:"jobUID,omitempty"`

	// PreparedAt is when validation and execution preparation completed.
	// +optional
	PreparedAt *metav1.Time `json:"preparedAt,omitempty"`

	// CommittedAt is when the controller committed to one Job creation attempt.
	// +optional
	CommittedAt *metav1.Time `json:"committedAt,omitempty"`

	// CreatedAt is when the controller persisted the created Job receipt.
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// TerminalResult is the persisted terminal Job result.
	// +optional
	TerminalResult RestoreExecutionResult `json:"terminalResult,omitempty"`

	// TerminalObservedAt is when the controller persisted the terminal Job result.
	// +optional
	TerminalObservedAt *metav1.Time `json:"terminalObservedAt,omitempty"`

	// FollowThroughCompletedAt is when post-restore recovery completed.
	// +optional
	FollowThroughCompletedAt *metav1.Time `json:"followThroughCompletedAt,omitempty"`
}

// RestoreSource defines where the snapshot comes from.
type RestoreSource struct {
	// Target reuses BackupTarget for storage connection details.
	// This includes endpoint, bucket, region, credentials, etc.
	Target BackupTarget `json:"target"`

	// Key is the full path to the snapshot object in the bucket.
	// For example, "clusters/prod/2025-10-14-120000.snap".
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// OpenBaoRestoreSpec defines the desired state for a restore operation.
// An OpenBaoRestore acts as a "job request" - it is immutable after creation.
type OpenBaoRestoreSpec struct {
	// Cluster is the name of the OpenBaoCluster to restore INTO.
	// Must exist in the same namespace as the OpenBaoRestore.
	// +kubebuilder:validation:MinLength=1
	Cluster string `json:"cluster"`

	// Source defines where the snapshot comes from.
	Source RestoreSource `json:"source"`

	// JWTAuthRole is the name of the JWT Auth role configured in OpenBao
	// for restore operations. When set, the restore executor will use JWT Auth
	// (projected ServiceAccount token) instead of a static token.
	//
	// The role must be configured in OpenBao and must grant the "update" capability on
	// sys/storage/raft/snapshot. To support force: true, it must also grant "update" on
	// sys/storage/raft/snapshot-force. The role must bind to the restore ServiceAccount
	// (<cluster-name>-restore-serviceaccount) in the cluster namespace.
	//
	// If this field is empty and the target OpenBaoCluster has OIDC enabled,
	// the operator will default to using the "openbao-operator-restore" role.
	// +optional
	JWTAuthRole string `json:"jwtAuthRole,omitempty"`

	// TokenSecretRef optionally references a Secret containing an OpenBao API
	// token to use for restore operations (fallback method).
	//
	// The Secret must exist in the same namespace as the OpenBaoRestore.
	// Cross-namespace references are not allowed for security reasons.
	//
	// The token must have permission to update sys/storage/raft/snapshot. To support
	// force: true, it must also have permission to update
	// sys/storage/raft/snapshot-force.
	//
	// If JWTAuthRole is set, this field is ignored in favor of JWT Auth.
	// +optional
	TokenSecretRef *corev1.LocalObjectReference `json:"tokenSecretRef,omitempty"`

	// Image is the container image to use for restore operations.
	// Defaults to the same image used for backup operations if not specified.
	// If the target OpenBaoCluster has image verification enabled, the operator will verify this image and pin the restore Job to the verified digest.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Image string `json:"image,omitempty"`

	// Force uses OpenBao's force-restore endpoint. This bypasses verification that
	// the snapshot is compatible with the target cluster's Shamir or auto-unseal
	// configuration. It also skips the controller checks that require the target
	// cluster to be initialized and not upgrading.
	//
	// Use this break-glass option only when the normal verified restore cannot run
	// and the snapshot source and target seal compatibility have been validated by
	// another trusted process.
	// +kubebuilder:default=false
	// +optional
	Force bool `json:"force,omitempty"`

	// OverrideOperationLock allows the restore controller to clear an active cluster
	// operation lock (upgrade/backup) and proceed with restore. This is a break-glass
	// escape hatch intended for disaster recovery.
	//
	// For safety, this requires force: true. When used, the controller emits a Warning
	// event and records a Condition on the OpenBaoRestore.
	//
	// +kubebuilder:default=false
	// +optional
	OverrideOperationLock bool `json:"overrideOperationLock,omitempty"`
}

// OpenBaoRestoreStatus defines the observed state of OpenBaoRestore.
type OpenBaoRestoreStatus struct {
	// Phase represents the current phase of the restore operation.
	// +kubebuilder:default=Pending
	Phase RestorePhase `json:"phase,omitempty"`

	// StartTime is when the restore operation started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the restore operation completed (success or failure).
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Execution records the stable operation identity and durable lifecycle receipts.
	// +optional
	Execution *RestoreExecutionStatus `json:"execution,omitempty"`

	// SnapshotKey is the key of the snapshot that was restored.
	// +optional
	SnapshotKey string `json:"snapshotKey,omitempty"`

	// SnapshotSize is the size of the restored snapshot in bytes.
	// +optional
	SnapshotSize int64 `json:"snapshotSize,omitempty"`

	// Message provides additional details about the current phase.
	// +optional
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations of the restore's state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=obrestore
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.cluster"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message",priority=1

// OpenBaoRestore represents a request to restore an OpenBao cluster from a snapshot.
// This resource is immutable after creation - it acts as a "job request".
type OpenBaoRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenBaoRestoreSpec   `json:"spec"`
	Status OpenBaoRestoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenBaoRestoreList contains a list of OpenBaoRestore.
type OpenBaoRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoRestore{}, &OpenBaoRestoreList{})
}
