// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package claimcontract

import (
	networkingv1 "k8s.io/api/networking/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// RenderedExecutionContract captures target-specific same-cluster execution inputs.
type RenderedExecutionContract struct {
	TargetNamespace string
	Cluster         RenderedCluster
	Unseal          RenderedUnseal
	Storage         RenderedStorage
	Runtime         ApprovedRuntime
	Observability   ApprovedObservability
	Bootstrap       RenderedBootstrap
	Exposure        RenderedExposure
	Backup          RenderedBackup
	Network         RenderedNetwork
	Lifecycle       ApprovedLifecycle
	Provenance      ApprovedServiceProvenance
}

// RenderedCluster captures rendered workload topology inputs.
type RenderedCluster struct {
	Version         string
	Replicas        int32
	ReadReplicas    int32
	SecurityProfile openbaov1alpha1.Profile
}

// RenderedUnseal captures rendered unseal posture required by the execution target.
type RenderedUnseal struct {
	Mode    UnsealPostureMode
	Transit *RenderedTransitUnseal
	Config  *openbaov1alpha1.UnsealConfig
}

// RenderedTransitUnseal captures rendered transit unseal inputs for same-cluster execution.
type RenderedTransitUnseal struct {
	Address               string
	KeyName               string
	MountPath             string
	Namespace             string
	TLSServerName         string
	CredentialsSecretName string
}

// SameClusterTransitUnsealDefaults captures installation-level transit unseal defaults
// used for first-wave hardened same-cluster claim materialization.
type SameClusterTransitUnsealDefaults struct {
	Address               string
	KeyName               string
	MountPath             string
	Namespace             string
	TLSServerName         string
	CredentialsSecretName string
}

// SameClusterNetworkDefaults captures installation-level network defaults used
// for same-cluster claim-managed workloads.
type SameClusterNetworkDefaults struct {
	APIServerCIDR        string
	APIServerEndpointIPs []string
	DNSEndpointIPs       []string
}

// ProjectedBootstrapArtifact captures one projected same-cluster bootstrap
// dependency artifact plus the bound content snapshot used to materialize it.
type ProjectedBootstrapArtifact struct {
	Ref           openbaov1alpha1.TypedObjectReference
	ConfigMapData map[string]string
	SecretData    map[string][]byte
}

// ProjectedBootstrapAuditSink captures one projected audit sink artifact plus
// the resolved audit path used to build the self-init request path.
type ProjectedBootstrapAuditSink struct {
	Artifact ProjectedBootstrapArtifact
	Path     string
}

// SameClusterBootstrapResolvedInputs captures locally resolved bootstrap dependency
// snapshots that can be projected into target-namespace artifacts for the
// same-cluster path.
type SameClusterBootstrapResolvedInputs struct {
	AuthMethodConfigs    map[string]ProjectedBootstrapArtifact
	PolicyBundleContents map[string]ProjectedBootstrapArtifact
	AuditDeviceSinks     map[string]ProjectedBootstrapAuditSink
}

// RenderedStorage captures rendered storage inputs.
type RenderedStorage struct {
	PrimarySize          string
	ReadReplicaSize      string
	ClassName            *string
	ReadReplicaClassName *string
	ACMECache            *openbaov1alpha1.ACMESharedCacheConfig
}

// RenderedBootstrap captures rendered bootstrap execution inputs.
type RenderedBootstrap struct {
	Mode                  openbaov1alpha1.OpenBaoBootstrapMode
	OperatorLifecycleAuth openbaov1alpha1.OpenBaoBootstrapLifecycleAuthSpec
	Auth                  *RenderedBootstrapAuthSpec
	SecretEngines         *openbaov1alpha1.OpenBaoBootstrapSecretEnginesSpec
	Policies              *RenderedBootstrapPoliciesSpec
	Audit                 *RenderedBootstrapAuditSpec
	ProfileRef            *openbaov1alpha1.LocalReference
}

// RenderedBootstrapAuthMethodSpec captures one rendered auth-method bootstrap entry.
type RenderedBootstrapAuthMethodSpec struct {
	Type          string
	Path          string
	ConfigFromRef *openbaov1alpha1.TypedObjectReference
}

// RenderedBootstrapAuthSpec captures rendered auth-method bootstrap inputs.
type RenderedBootstrapAuthSpec struct {
	Methods []RenderedBootstrapAuthMethodSpec
}

// RenderedBootstrapPolicyBundleSpec captures one rendered bootstrap policy bundle.
type RenderedBootstrapPolicyBundleSpec struct {
	Name           string
	ContentFromRef openbaov1alpha1.TypedObjectReference
}

// RenderedBootstrapPoliciesSpec captures rendered bootstrap policy inputs.
type RenderedBootstrapPoliciesSpec struct {
	Bundles []RenderedBootstrapPolicyBundleSpec
}

// RenderedBootstrapAuditDeviceSpec captures one rendered bootstrap audit device.
type RenderedBootstrapAuditDeviceSpec struct {
	Type        string
	SinkFromRef *openbaov1alpha1.TypedObjectReference
	Path        string
}

// RenderedBootstrapAuditSpec captures rendered audit-device bootstrap inputs.
type RenderedBootstrapAuditSpec struct {
	Devices []RenderedBootstrapAuditDeviceSpec
}

// RenderedExposure captures rendered exposure execution inputs.
type RenderedExposure struct {
	PublishMode              openbaov1alpha1.OpenBaoExposurePublishMode
	HostnamePolicy           openbaov1alpha1.OpenBaoExposureHostnamePolicySpec
	TLSPolicy                *openbaov1alpha1.OpenBaoExposureTLSPolicySpec
	EntrypointRef            *openbaov1alpha1.LocalReference
	Entrypoint               *RenderedExposureEntrypoint
	IngressPolicyRef         *openbaov1alpha1.LocalReference
	Ingress                  *RenderedExposureIngress
	Routing                  *openbaov1alpha1.OpenBaoExposureRoutingSpec
	GatewayAnnotations       map[string]string
	ServicePolicy            *openbaov1alpha1.OpenBaoExposureServicePolicySpec
	ReadReplicaServicePolicy *openbaov1alpha1.OpenBaoExposureReadReplicaServicePolicySpec
}

// RenderedExposureEntrypoint captures concrete entrypoint execution inputs.
type RenderedExposureEntrypoint struct {
	Ref            *RenderedBoundReference
	Mode           openbaov1alpha1.OpenBaoEntrypointMode
	ObjectRef      openbaov1alpha1.OpenBaoEntrypointObjectReference
	ListenerPolicy *openbaov1alpha1.OpenBaoEntrypointListenerPolicySpec
}

// RenderedExposureIngress captures concrete ingress execution inputs.
type RenderedExposureIngress struct {
	PolicyRef                 *RenderedBoundReference
	ClassName                 string
	PathType                  openbaov1alpha1.IngressPathType
	Annotations               map[string]string
	BackendTLSPublicationMode openbaov1alpha1.OpenBaoIngressBackendTLSPublicationMode
	ReadinessMode             openbaov1alpha1.IngressReadinessMode
}

// RenderedNetwork captures rendered execution-time network-policy inputs.
type RenderedNetwork struct {
	RequiredEgressRules  []networkingv1.NetworkPolicyEgressRule
	APIServerCIDR        string
	APIServerEndpointIPs []string
	DNSNamespace         string
	DNSEndpointIPs       []string
	EgressRules          []networkingv1.NetworkPolicyEgressRule
	IngressRules         []networkingv1.NetworkPolicyIngressRule
	TrustedIngressPeers  []networkingv1.NetworkPolicyPeer
}

// RenderedBoundReference retains immutable dependency identity in rendered execution inputs.
type RenderedBoundReference struct {
	Name string
	UID  string
}

// RenderedBackupBackend captures concrete backup backend execution inputs.
type RenderedBackupBackend struct {
	Driver              openbaov1alpha1.OpenBaoBackupBackendDriver
	Provider            openbaov1alpha1.OpenBaoObjectStorageProvider
	Endpoint            string
	Region              string
	UsePathStyle        bool
	GCSProject          string
	AzureStorageAccount string
	AzureContainer      string
	InsecureSkipVerify  bool
	RequiredEgressRules []networkingv1.NetworkPolicyEgressRule
}

// RenderedBackupAuth captures concrete backup storage-auth execution inputs.
type RenderedBackupAuth struct {
	Mode                  openbaov1alpha1.OpenBaoBackupAuthMode
	StaticCredentialsName string
	WorkloadIdentity      *openbaov1alpha1.WorkloadIdentityConfig
	RoleARN               string
}

// RenderedBackupTransfer captures concrete transfer tuning execution inputs.
type RenderedBackupTransfer struct {
	PartSize    int64
	Concurrency int32
}

// RenderedBackup captures rendered backup execution inputs.
type RenderedBackup struct {
	Schedule           string
	Retention          *openbaov1alpha1.BackupRetention
	TargetRef          *RenderedBoundReference
	BackendRef         *RenderedBoundReference
	AuthProfileRef     *RenderedBoundReference
	TransferProfileRef *RenderedBoundReference
	Location           string
	Partition          string
	KeyPrefix          string
	Backend            *RenderedBackupBackend
	Auth               *RenderedBackupAuth
	Transfer           *RenderedBackupTransfer
	ProfileRef         openbaov1alpha1.LocalReference
}

// RenderSameClusterExecutionContract renders target-specific same-cluster execution inputs.
func RenderSameClusterExecutionContract(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	target *openbaov1alpha1.NamespacedReference,
	approved *ApprovedServiceContract,
	catalog *CatalogBundle,
	transitDefaults SameClusterTransitUnsealDefaults,
	bootstrapInputs SameClusterBootstrapResolvedInputs,
) (*RenderedExecutionContract, ValidationResult) {
	if claim == nil {
		return nil, ValidationResult{Valid: false, Reason: openbaov1alpha1.ReasonInvalid, Message: "OpenBaoClusterClaim is required to render execution inputs."}
	}
	if target == nil || target.Namespace == "" {
		return nil, ValidationResult{Valid: false, Reason: openbaov1alpha1.ReasonPending, Message: "Same-cluster target namespace is required to render execution inputs."}
	}
	if approved == nil {
		return nil, ValidationResult{Valid: false, Reason: openbaov1alpha1.ReasonPending, Message: "Approved service contract is required to render execution inputs."}
	}
	if catalog == nil || catalog.ServiceProfile == nil || catalog.ExposureClass == nil || catalog.BackupProfile == nil {
		return nil, ValidationResult{Valid: false, Reason: openbaov1alpha1.ReasonPending, Message: "Immutable catalog inputs are required to render execution inputs."}
	}
	if catalog.ServiceProfile.Spec.Bootstrap.ProfileRef != nil && catalog.BootstrapProfile == nil {
		return nil, ValidationResult{Valid: false, Reason: openbaov1alpha1.ReasonPending, Message: "OpenBaoBootstrapProfile is required to render execution inputs."}
	}

	renderedBackup, validation := renderBackup(claim, target, approved, catalog)
	if !validation.Valid {
		return nil, validation
	}
	renderedEntrypoint, validation := renderEntrypoint(catalog.ExposureClass, catalog.Entrypoint)
	if !validation.Valid {
		return nil, validation
	}
	renderedIngress, validation := renderIngress(catalog.ExposureClass, catalog.Entrypoint, catalog.IngressPolicy)
	if !validation.Valid {
		return nil, validation
	}
	hostnamePolicy, validation := renderedHostnamePolicy(claim, catalog.ExposureClass.Spec.HostnamePolicy)
	if !validation.Valid {
		return nil, validation
	}

	rendered := &RenderedExecutionContract{
		TargetNamespace: target.Namespace,
		Cluster: RenderedCluster{
			Version:         approved.Cluster.Version,
			Replicas:        approved.Cluster.Voters,
			ReadReplicas:    approved.Cluster.ReadReplicas,
			SecurityProfile: approved.Cluster.SecurityProfile,
		},
		Unseal: RenderedUnseal{
			Mode: approved.Unseal.Mode,
		},
		Storage: RenderedStorage{
			PrimarySize:          approved.Storage.PrimarySize,
			ReadReplicaSize:      approved.Storage.ReadReplicaSize,
			ClassName:            cloneStringPtr(approved.Storage.PrimaryClassName),
			ReadReplicaClassName: cloneStringPtr(approved.Storage.ReadReplicaClassName),
			ACMECache:            cloneACMESharedCacheConfig(approved.Storage.ACMECache),
		},
		Runtime:       cloneApprovedRuntime(approved.Runtime),
		Observability: cloneApprovedObservability(approved.Observability),
		Bootstrap: RenderedBootstrap{
			Mode:       approved.Bootstrap.Mode,
			ProfileRef: approved.Bootstrap.ProfileRef,
		},
		Exposure: RenderedExposure{
			PublishMode:              catalog.ExposureClass.Spec.PublishMode,
			HostnamePolicy:           hostnamePolicy,
			TLSPolicy:                cloneTLSPolicy(catalog.ExposureClass.Spec.TLSPolicy),
			EntrypointRef:            cloneLocalReference(catalog.ExposureClass.Spec.EntrypointRef),
			Entrypoint:               renderedEntrypoint,
			IngressPolicyRef:         cloneLocalReference(catalog.ExposureClass.Spec.IngressPolicyRef),
			Ingress:                  renderedIngress,
			Routing:                  cloneRouting(catalog.ExposureClass.Spec.Routing),
			GatewayAnnotations:       cloneStringMap(catalog.ExposureClass.Spec.GatewayAnnotations),
			ServicePolicy:            cloneServicePolicy(catalog.ExposureClass.Spec.ServicePolicy),
			ReadReplicaServicePolicy: cloneReadReplicaServicePolicy(catalog.ExposureClass.Spec.ReadReplicaServicePolicy),
		},
		Backup:     renderedBackup,
		Network:    renderedNetwork(approved.Network, renderedBackup),
		Lifecycle:  approved.Lifecycle,
		Provenance: approved.Provenance,
	}
	if approved.Unseal.Config != nil {
		rendered.Unseal.Config = approved.Unseal.Config.DeepCopy()
	}
	if catalog.BootstrapProfile != nil {
		rendered.Bootstrap.OperatorLifecycleAuth = catalog.BootstrapProfile.Spec.OperatorLifecycleAuth
		renderedAuth, validation := renderBootstrapAuth(catalog.BootstrapProfile.Spec.Auth, bootstrapInputs)
		if !validation.Valid {
			return nil, validation
		}
		rendered.Bootstrap.Auth = renderedAuth
		rendered.Bootstrap.SecretEngines = cloneBootstrapSecretEngines(catalog.BootstrapProfile.Spec.SecretEngines)
		renderedPolicies, validation := renderBootstrapPolicies(catalog.BootstrapProfile.Spec.Policies, bootstrapInputs)
		if !validation.Valid {
			return nil, validation
		}
		rendered.Bootstrap.Policies = renderedPolicies
		renderedAudit, validation := renderBootstrapAudit(catalog.BootstrapProfile.Spec.Audit, bootstrapInputs)
		if !validation.Valid {
			return nil, validation
		}
		rendered.Bootstrap.Audit = renderedAudit
	}
	if approved.Unseal.Mode == UnsealPostureModeExternal && rendered.Unseal.Config == nil {
		validation := applySameClusterTransitUnseal(rendered, transitDefaults)
		if !validation.Valid {
			return nil, validation
		}
	}

	return rendered, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered execution contract has been produced for the same-cluster materialization path.",
	}
}
