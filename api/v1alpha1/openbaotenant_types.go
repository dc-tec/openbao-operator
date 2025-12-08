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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenBaoTenantSpec defines the desired state of OpenBaoTenant.
type OpenBaoTenantSpec struct {
	// TargetNamespace is the name of the namespace to provision with tenant RBAC.
	// The Provisioner will create Role and RoleBinding resources in this namespace
	// to grant the OpenBaoCluster controller permission to manage OpenBaoCluster
	// resources in that namespace.
	// +kubebuilder:validation:MinLength=1
	TargetNamespace string `json:"targetNamespace"`
}

// OpenBaoTenantStatus defines the observed state of OpenBaoTenant.
type OpenBaoTenantStatus struct {
	// Provisioned indicates if the RBAC has been successfully applied to the target namespace.
	// +optional
	Provisioned bool `json:"provisioned"`

	// LastError reports any issues finding the namespace or applying RBAC.
	// +optional
	LastError string `json:"lastError,omitempty"`

	// Conditions represent the latest available observations of the tenant's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=obt
// +kubebuilder:printcolumn:name="Target Namespace",type="string",JSONPath=".spec.targetNamespace"
// +kubebuilder:printcolumn:name="Provisioned",type="boolean",JSONPath=".status.provisioned"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoTenant is the Schema for the openbaotenants API.
// OpenBaoTenant is a governance CRD that explicitly declares which namespace
// should be provisioned with tenant RBAC. This replaces the previous label-based
// approach (openbao.org/tenant=true) to improve security by eliminating the need
// for the Provisioner to have list/watch permissions on namespaces.
type OpenBaoTenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenBaoTenantSpec   `json:"spec,omitempty"`
	Status OpenBaoTenantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenBaoTenantList contains a list of OpenBaoTenant.
type OpenBaoTenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoTenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoTenant{}, &OpenBaoTenantList{})
}
