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

// OpenBaoBootstrapLifecycleAuthMode identifies the lifecycle-auth posture for bootstrap.
// +kubebuilder:validation:Enum=JWT
type OpenBaoBootstrapLifecycleAuthMode string

const (
	// OpenBaoBootstrapLifecycleAuthModeJWT uses JWT auth for operator lifecycle operations.
	OpenBaoBootstrapLifecycleAuthModeJWT OpenBaoBootstrapLifecycleAuthMode = "JWT"
)

// OpenBaoBootstrapLifecycleJWTSpec configures JWT-based lifecycle auth.
type OpenBaoBootstrapLifecycleJWTSpec struct {
	// Audience is the lifecycle JWT audience the initialized cluster should accept.
	// +kubebuilder:validation:MinLength=1
	Audience string `json:"audience"`
}

// OpenBaoBootstrapLifecycleAuthSpec defines lifecycle-auth posture for bootstrap.
type OpenBaoBootstrapLifecycleAuthSpec struct {
	// Mode identifies the lifecycle-auth mode.
	Mode OpenBaoBootstrapLifecycleAuthMode `json:"mode"`
	// JWT carries JWT-specific lifecycle-auth configuration.
	// +optional
	JWT *OpenBaoBootstrapLifecycleJWTSpec `json:"jwt,omitempty"`
}

// OpenBaoBootstrapAuthMethodSpec defines one auth method bootstrap entry.
type OpenBaoBootstrapAuthMethodSpec struct {
	// Type is the auth method type to enable.
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
	// Path is the mount path for the auth method.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// ConfigRef identifies platform-managed configuration for the auth method.
	// +optional
	ConfigRef *TypedObjectReference `json:"configRef,omitempty"`
}

// OpenBaoBootstrapAuthSpec defines auth method bootstrap configuration.
type OpenBaoBootstrapAuthSpec struct {
	// Methods lists auth methods that should exist after bootstrap.
	// +optional
	Methods []OpenBaoBootstrapAuthMethodSpec `json:"methods,omitempty"`
}

// OpenBaoBootstrapSecretEngineMountSpec defines one secret-engine mount entry.
type OpenBaoBootstrapSecretEngineMountSpec struct {
	// Type is the secret-engine type to enable.
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
	// Path is the mount path for the secret engine.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
}

// OpenBaoBootstrapSecretEnginesSpec defines secret-engine bootstrap configuration.
type OpenBaoBootstrapSecretEnginesSpec struct {
	// Mounts lists secret-engine mounts that should exist after bootstrap.
	// +optional
	Mounts []OpenBaoBootstrapSecretEngineMountSpec `json:"mounts,omitempty"`
}

// OpenBaoBootstrapPolicyBundleSpec defines one policy bundle entry.
type OpenBaoBootstrapPolicyBundleSpec struct {
	// Name identifies the bundle inside the bootstrap profile.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// ContentRef identifies policy content to apply during bootstrap.
	ContentRef TypedObjectReference `json:"contentRef"`
}

// OpenBaoBootstrapPoliciesSpec defines policy bootstrap configuration.
type OpenBaoBootstrapPoliciesSpec struct {
	// Bundles lists policy bundles that should exist after bootstrap.
	// +optional
	Bundles []OpenBaoBootstrapPolicyBundleSpec `json:"bundles,omitempty"`
}

// OpenBaoBootstrapAuditDeviceSpec defines one audit device bootstrap entry.
type OpenBaoBootstrapAuditDeviceSpec struct {
	// Type is the audit device type to enable.
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
	// SinkRef identifies platform-managed sink wiring for the audit device.
	// +optional
	SinkRef *TypedObjectReference `json:"sinkRef,omitempty"`
}

// OpenBaoBootstrapAuditSpec defines audit bootstrap configuration.
type OpenBaoBootstrapAuditSpec struct {
	// Devices lists audit devices that should exist after bootstrap.
	// +optional
	Devices []OpenBaoBootstrapAuditDeviceSpec `json:"devices,omitempty"`
}

// OpenBaoBootstrapProfileSpec defines the desired state of OpenBaoBootstrapProfile.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoBootstrapProfileSpec struct {
	// OperatorLifecycleAuth defines operator lifecycle-auth posture for initialized clusters.
	OperatorLifecycleAuth OpenBaoBootstrapLifecycleAuthSpec `json:"operatorLifecycleAuth"`
	// Auth defines auth methods that should exist after bootstrap.
	// +optional
	Auth *OpenBaoBootstrapAuthSpec `json:"auth,omitempty"`
	// SecretEngines defines secret-engine mounts that should exist after bootstrap.
	// +optional
	SecretEngines *OpenBaoBootstrapSecretEnginesSpec `json:"secretEngines,omitempty"`
	// Policies defines policy bundles that should exist after bootstrap.
	// +optional
	Policies *OpenBaoBootstrapPoliciesSpec `json:"policies,omitempty"`
	// Audit defines audit devices that should exist after bootstrap.
	// +optional
	Audit *OpenBaoBootstrapAuditSpec `json:"audit,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Lifecycle Auth",type="string",JSONPath=".spec.operatorLifecycleAuth.mode"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoBootstrapProfile is the immutable platform-owned bootstrap bundle catalog object.
type OpenBaoBootstrapProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoBootstrapProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoBootstrapProfileList contains a list of OpenBaoBootstrapProfile.
type OpenBaoBootstrapProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoBootstrapProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoBootstrapProfile{}, &OpenBaoBootstrapProfileList{})
}
