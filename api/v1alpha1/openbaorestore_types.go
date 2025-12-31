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
)

// RestorePhase represents the current phase of a restore operation.
// +kubebuilder:validation:Enum=Pending;Validating;Running;Completed;Failed
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
)

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
	// sys/storage/raft/snapshot-force. The role must bind to the restore ServiceAccount
	// (<cluster-name>-restore-serviceaccount) in the cluster namespace.
	// +optional
	JWTAuthRole string `json:"jwtAuthRole,omitempty"`

	// TokenSecretRef optionally references a Secret containing an OpenBao API
	// token to use for restore operations (fallback method).
	//
	// The token must have permission to update sys/storage/raft/snapshot-force.
	//
	// If JWTAuthRole is set, this field is ignored in favor of JWT Auth.
	// +optional
	TokenSecretRef *corev1.SecretReference `json:"tokenSecretRef,omitempty"`

	// ExecutorImage is the container image to use for restore operations.
	// Defaults to the same image used for backup operations if not specified.
	// +kubebuilder:validation:MinLength=1
	// +optional
	ExecutorImage string `json:"executorImage,omitempty"`

	// Force allows restore even if the cluster appears unhealthy.
	// This is required for disaster recovery scenarios where the cluster
	// may be in a degraded state.
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

	Spec   OpenBaoRestoreSpec   `json:"spec,omitempty"`
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
