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

// LocalReference identifies another object by name.
type LocalReference struct {
	// Name is the referenced object name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// NamespacedReference identifies another object by namespace and name.
type NamespacedReference struct {
	// Namespace is the referenced object namespace.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
	// Name is the referenced object name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// TypedObjectReference identifies another object by kind, namespace, and name.
type TypedObjectReference struct {
	// Kind is the referenced object kind.
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`
	// Namespace is the referenced object namespace when applicable.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// Name is the referenced object name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ConditionReason identifies low-cardinality status reasons shared across modules.
type ConditionReason string

const (
	// ReasonFeatureDisabled indicates the feature gate is disabled.
	ReasonFeatureDisabled ConditionReason = "FeatureDisabled"
	// ReasonPending indicates reconciliation or readiness is still pending.
	ReasonPending ConditionReason = "Pending"
	// ReasonReady indicates the observed state is ready.
	ReasonReady ConditionReason = "Ready"
	// ReasonAccepted indicates requested intent has been accepted.
	ReasonAccepted ConditionReason = "Accepted"
	// ReasonPlacementPending indicates placement is not yet resolved.
	ReasonPlacementPending ConditionReason = "PlacementPending"
	// ReasonConsumed indicates a one-shot resource has already been consumed.
	ReasonConsumed ConditionReason = "Consumed"
	// ReasonExpired indicates a time-bound resource has expired.
	ReasonExpired ConditionReason = "Expired"
	// ReasonInvalid indicates the current state does not satisfy validation.
	ReasonInvalid ConditionReason = "Invalid"
)
