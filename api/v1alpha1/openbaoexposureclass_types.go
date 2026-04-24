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

// OpenBaoExposurePublishMode identifies the service publication posture.
// +kubebuilder:validation:Enum=ClusterInternal;Ingress;Gateway
type OpenBaoExposurePublishMode string

const (
	// OpenBaoExposurePublishModeClusterInternal publishes the service only inside the cluster.
	OpenBaoExposurePublishModeClusterInternal OpenBaoExposurePublishMode = "ClusterInternal"
	// OpenBaoExposurePublishModeIngress publishes the service through ingress-style routing.
	OpenBaoExposurePublishModeIngress OpenBaoExposurePublishMode = "Ingress"
	// OpenBaoExposurePublishModeGateway publishes the service through Gateway API routing.
	OpenBaoExposurePublishModeGateway OpenBaoExposurePublishMode = "Gateway"
)

// OpenBaoExposureHostnamePolicyMode identifies hostname handling posture.
// +kubebuilder:validation:Enum=Generated;Fixed
type OpenBaoExposureHostnamePolicyMode string

const (
	// OpenBaoExposureHostnamePolicyModeGenerated derives hostnames from platform policy.
	OpenBaoExposureHostnamePolicyModeGenerated OpenBaoExposureHostnamePolicyMode = "Generated"
	// OpenBaoExposureHostnamePolicyModeFixed uses a fixed explicit hostname.
	OpenBaoExposureHostnamePolicyModeFixed OpenBaoExposureHostnamePolicyMode = "Fixed"
)

// OpenBaoExposureTLSMode identifies TLS posture for published traffic.
// +kubebuilder:validation:Enum=External;OperatorManaged;ACME
type OpenBaoExposureTLSMode string

const (
	// OpenBaoExposureTLSModeExternal uses externally managed certificate material.
	OpenBaoExposureTLSModeExternal OpenBaoExposureTLSMode = "External"
	// OpenBaoExposureTLSModeOperatorManaged uses operator-managed certificate material.
	OpenBaoExposureTLSModeOperatorManaged OpenBaoExposureTLSMode = "OperatorManaged"
	// OpenBaoExposureTLSModeACME uses OpenBao native ACME for listener certificates.
	OpenBaoExposureTLSModeACME OpenBaoExposureTLSMode = "ACME"
)

// OpenBaoExposureTLSMinimumVersion identifies the minimum TLS version to require.
// +kubebuilder:validation:Enum=TLS12;TLS13
type OpenBaoExposureTLSMinimumVersion string

const (
	// OpenBaoExposureTLSMinimumVersionTLS12 requires TLS 1.2 or newer.
	OpenBaoExposureTLSMinimumVersionTLS12 OpenBaoExposureTLSMinimumVersion = "TLS12"
	// OpenBaoExposureTLSMinimumVersionTLS13 requires TLS 1.3 or newer.
	OpenBaoExposureTLSMinimumVersionTLS13 OpenBaoExposureTLSMinimumVersion = "TLS13"
)

// OpenBaoExposureServiceType identifies how the backing Service should be exposed.
// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
type OpenBaoExposureServiceType string

const (
	// OpenBaoExposureServiceTypeClusterIP uses an internal-only ClusterIP Service.
	OpenBaoExposureServiceTypeClusterIP OpenBaoExposureServiceType = "ClusterIP"
	// OpenBaoExposureServiceTypeNodePort uses a NodePort Service.
	OpenBaoExposureServiceTypeNodePort OpenBaoExposureServiceType = "NodePort"
	// OpenBaoExposureServiceTypeLoadBalancer uses a LoadBalancer Service.
	OpenBaoExposureServiceTypeLoadBalancer OpenBaoExposureServiceType = "LoadBalancer"
)

// OpenBaoExposureBackendTLSMode identifies backend TLS posture between entrypoint and workload.
// +kubebuilder:validation:Enum=Required;Disabled
type OpenBaoExposureBackendTLSMode string

const (
	// OpenBaoExposureBackendTLSModeRequired requires TLS between entrypoint and workload.
	OpenBaoExposureBackendTLSModeRequired OpenBaoExposureBackendTLSMode = "Required"
	// OpenBaoExposureBackendTLSModeDisabled disables backend TLS requirements.
	OpenBaoExposureBackendTLSModeDisabled OpenBaoExposureBackendTLSMode = "Disabled"
)

// OpenBaoExposureHostnamePolicySpec defines hostname generation posture.
type OpenBaoExposureHostnamePolicySpec struct {
	// Mode identifies hostname policy mode.
	Mode OpenBaoExposureHostnamePolicyMode `json:"mode"`
	// DomainSuffix is the generated-hostname suffix when Mode is Generated.
	// +optional
	DomainSuffix string `json:"domainSuffix,omitempty"`
	// Value is the fixed hostname when Mode is Fixed.
	// +optional
	Value string `json:"value,omitempty"`
	// Claim allows bounded tenant-provided hostnames.
	// +optional
	Claim *OpenBaoExposureClaimHostnamePolicySpec `json:"claim,omitempty"`
}

// OpenBaoExposureClaimHostnamePolicySpec defines bounded tenant-provided
// hostname policy.
type OpenBaoExposureClaimHostnamePolicySpec struct {
	// Enabled allows claims to request a hostname through service parameters.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// AllowedSuffixes limits tenant-provided hostnames to these DNS suffixes.
	// When omitted, DomainSuffix is used as the only allowed suffix.
	// +optional
	AllowedSuffixes []string `json:"allowedSuffixes,omitempty"`
}

// OpenBaoExposureACMEPolicySpec defines platform-owned native OpenBao ACME posture.
// +kubebuilder:validation:XValidation:rule="!(has(self.domain) && has(self.domains) && size(self.domains) > 0)",message="acme.domain and acme.domains are mutually exclusive; use only one"
type OpenBaoExposureACMEPolicySpec struct {
	// DirectoryURL is the ACME directory URL.
	// +kubebuilder:validation:MinLength=1
	DirectoryURL string `json:"directoryURL"`
	// Domain is the domain name for which to obtain the certificate.
	// Deprecated: use Domains to request a certificate with multiple SANs.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Domain string `json:"domain,omitempty"`
	// Domains is the list of domain names for which to obtain the certificate.
	//
	// When empty, the operator relies on OpenBaoCluster ACME defaults for private
	// cluster-internal ACME deployments.
	// +kubebuilder:validation:MinItems=1
	// +optional
	Domains []string `json:"domains,omitempty"`
	// Email is the email address to use for ACME registration.
	// +optional
	Email string `json:"email,omitempty"`
}

// OpenBaoExposureTLSPolicySpec defines TLS posture for published traffic.
// +kubebuilder:validation:XValidation:rule="self.mode != 'ACME' || has(self.acme)",message="tlsPolicy.acme is required when mode is ACME"
// +kubebuilder:validation:XValidation:rule="self.mode == 'ACME' || !has(self.acme)",message="tlsPolicy.acme is only supported when mode is ACME"
type OpenBaoExposureTLSPolicySpec struct {
	// Mode identifies TLS posture.
	Mode OpenBaoExposureTLSMode `json:"mode"`
	// CertificateRef identifies externally managed certificate material when required.
	// +optional
	CertificateRef *TypedObjectReference `json:"certificateRef,omitempty"`
	// ACME configures native OpenBao ACME listener TLS when Mode is ACME.
	// +optional
	ACME *OpenBaoExposureACMEPolicySpec `json:"acme,omitempty"`
	// MinVersion identifies the minimum allowed TLS version.
	// +kubebuilder:default=TLS12
	// +optional
	MinVersion OpenBaoExposureTLSMinimumVersion `json:"minVersion,omitempty"`
}

// OpenBaoExposureRoutingSpec defines route-shape posture for published traffic.
type OpenBaoExposureRoutingSpec struct {
	// Path is the HTTP path prefix when the route type supports it.
	// +optional
	Path string `json:"path,omitempty"`
	// TLSPassthrough identifies whether TLS should remain encrypted through the entrypoint.
	// +optional
	TLSPassthrough bool `json:"tlsPassthrough,omitempty"`
}

// OpenBaoExposureServicePolicySpec defines backing Service posture.
type OpenBaoExposureServicePolicySpec struct {
	// Type identifies the backing Service type.
	// +kubebuilder:default=ClusterIP
	// +optional
	Type OpenBaoExposureServiceType `json:"type,omitempty"`
	// BackendTLSMode identifies backend TLS posture.
	// +kubebuilder:default=Required
	// +optional
	BackendTLSMode OpenBaoExposureBackendTLSMode `json:"backendTLSMode,omitempty"`
	// Annotations are additional annotations to apply to the backing Service.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// OpenBaoExposureReadReplicaServicePolicySpec defines read-replica Service
// exposure owned by the catalog.
type OpenBaoExposureReadReplicaServicePolicySpec struct {
	// Enabled controls whether a read-replica Service is rendered.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Type controls the rendered read-replica Service type.
	// +kubebuilder:default=ClusterIP
	// +optional
	Type OpenBaoExposureServiceType `json:"type,omitempty"`
	// Annotations are copied to the rendered read-replica Service.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// OpenBaoExposureClassSpec defines the desired state of OpenBaoExposureClass.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
// +kubebuilder:validation:XValidation:rule="self.publishMode != 'Ingress' || has(self.ingressPolicyRef)",message="spec.ingressPolicyRef is required when publishMode is Ingress"
// +kubebuilder:validation:XValidation:rule="self.publishMode == 'Ingress' || !has(self.ingressPolicyRef)",message="spec.ingressPolicyRef is only supported when publishMode is Ingress"
type OpenBaoExposureClassSpec struct {
	// PublishMode identifies how the service should be published.
	PublishMode OpenBaoExposurePublishMode `json:"publishMode"`
	// HostnamePolicy defines hostname posture for published traffic.
	HostnamePolicy OpenBaoExposureHostnamePolicySpec `json:"hostnamePolicy"`
	// TLSPolicy defines TLS posture for published traffic.
	// +optional
	TLSPolicy *OpenBaoExposureTLSPolicySpec `json:"tlsPolicy,omitempty"`
	// EntrypointRef identifies the reusable platform-managed entrypoint when needed.
	// +optional
	EntrypointRef *LocalReference `json:"entrypointRef,omitempty"`
	// IngressPolicyRef identifies the reusable platform-managed ingress policy
	// when publishMode is Ingress.
	// +optional
	IngressPolicyRef *LocalReference `json:"ingressPolicyRef,omitempty"`
	// Routing defines route-shape posture.
	// +optional
	Routing *OpenBaoExposureRoutingSpec `json:"routing,omitempty"`
	// GatewayAnnotations are copied to generated Gateway API resources.
	// +optional
	GatewayAnnotations map[string]string `json:"gatewayAnnotations,omitempty"`
	// ServicePolicy defines backing Service posture.
	// +optional
	ServicePolicy *OpenBaoExposureServicePolicySpec `json:"servicePolicy,omitempty"`
	// ReadReplicaServicePolicy defines backing Service posture for read replicas.
	// +optional
	ReadReplicaServicePolicy *OpenBaoExposureReadReplicaServicePolicySpec `json:"readReplicaServicePolicy,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Mode",type="string",JSONPath=".spec.publishMode"
// +kubebuilder:printcolumn:name="Entrypoint",type="string",JSONPath=".spec.entrypointRef.name"
// +kubebuilder:printcolumn:name="IngressPolicy",type="string",JSONPath=".spec.ingressPolicyRef.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoExposureClass is the immutable platform-owned exposure catalog object.
type OpenBaoExposureClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoExposureClassSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoExposureClassList contains a list of OpenBaoExposureClass.
type OpenBaoExposureClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoExposureClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoExposureClass{}, &OpenBaoExposureClassList{})
}
