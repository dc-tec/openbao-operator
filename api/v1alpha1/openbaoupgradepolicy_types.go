/*
Copyright 2026.

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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// OpenBaoUpgradePolicySpec defines platform-owned upgrade behavior for
// catalog-backed OpenBao clusters.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoUpgradePolicySpec struct {
	// BlueGreen defines bounded blue/green behavior. It is applied when the
	// service profile selects the BlueGreen upgrade strategy.
	// +optional
	BlueGreen *OpenBaoUpgradePolicyBlueGreenSpec `json:"blueGreen,omitempty"`
}

// OpenBaoUpgradePolicyBlueGreenSpec defines the catalog-safe subset of
// blue/green upgrade settings.
type OpenBaoUpgradePolicyBlueGreenSpec struct {
	// AutoPromote controls whether the controller promotes the candidate
	// deployment automatically after verification.
	// +optional
	AutoPromote *bool `json:"autoPromote,omitempty"`

	// MinSyncDuration is the minimum time the candidate should stay in sync
	// before promotion.
	// +optional
	MinSyncDuration string `json:"minSyncDuration,omitempty"`

	// MaxJobFailures limits verification job failures before the upgrade is
	// marked failed.
	// +optional
	MaxJobFailures *int32 `json:"maxJobFailures,omitempty"`

	// AutoRollback controls rollback behavior when blue/green verification or
	// promotion fails.
	// +optional
	AutoRollback *OpenBaoUpgradePolicyAutoRollbackSpec `json:"autoRollback,omitempty"`
}

// OpenBaoUpgradePolicyAutoRollbackSpec defines the catalog-safe subset of
// automatic rollback settings.
type OpenBaoUpgradePolicyAutoRollbackSpec struct {
	// Enabled controls whether automatic rollback is enabled.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// OnJobFailure rolls back when verification jobs exceed MaxJobFailures.
	// +optional
	OnJobFailure *bool `json:"onJobFailure,omitempty"`

	// OnValidationFailure rolls back when validation fails.
	// +optional
	OnValidationFailure *bool `json:"onValidationFailure,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoUpgradePolicy is the immutable platform-owned upgrade-policy catalog object.
type OpenBaoUpgradePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoUpgradePolicySpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoUpgradePolicyList contains a list of OpenBaoUpgradePolicy.
type OpenBaoUpgradePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoUpgradePolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoUpgradePolicy{}, &OpenBaoUpgradePolicyList{})
}
