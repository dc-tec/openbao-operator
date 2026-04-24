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

import (
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenBaoNetworkProfileSpec defines platform-owned network dependencies for a
// catalog-backed OpenBao cluster.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoNetworkProfileSpec struct {
	// APIServerCIDR allows egress to the Kubernetes API server by CIDR.
	// +optional
	APIServerCIDR string `json:"apiServerCIDR,omitempty"`

	// APIServerEndpointIPs allows egress to Kubernetes API server endpoint IPs.
	// +optional
	APIServerEndpointIPs []string `json:"apiServerEndpointIPs,omitempty"`

	// DNSNamespace is the namespace containing cluster DNS endpoints.
	// +optional
	DNSNamespace string `json:"dnsNamespace,omitempty"`

	// DNSEndpointIPs allows egress to explicit DNS endpoint IPs.
	// +optional
	DNSEndpointIPs []string `json:"dnsEndpointIPs,omitempty"`

	// EgressRules are additional platform-approved NetworkPolicy egress rules.
	// +optional
	EgressRules []networkingv1.NetworkPolicyEgressRule `json:"egressRules,omitempty"`

	// IngressRules are additional platform-approved NetworkPolicy ingress rules.
	// +optional
	IngressRules []networkingv1.NetworkPolicyIngressRule `json:"ingressRules,omitempty"`

	// TrustedIngressPeers are platform-approved peers allowed to reach OpenBao.
	// +optional
	TrustedIngressPeers []networkingv1.NetworkPolicyPeer `json:"trustedIngressPeers,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="DNS Namespace",type="string",JSONPath=".spec.dnsNamespace"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoNetworkProfile is the immutable platform-owned network dependency catalog object.
type OpenBaoNetworkProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoNetworkProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoNetworkProfileList contains a list of OpenBaoNetworkProfile.
type OpenBaoNetworkProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoNetworkProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoNetworkProfile{}, &OpenBaoNetworkProfileList{})
}
