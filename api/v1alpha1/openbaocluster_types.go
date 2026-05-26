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
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeletionPolicy defines what happens to underlying resources when the CR is deleted.
// +kubebuilder:validation:Enum=Retain;DeletePVCs;DeleteAll
type DeletionPolicy string

const (
	// OpenBaoClusterFinalizer is the finalizer used to ensure cleanup logic
	// runs before an OpenBaoCluster is fully deleted.
	OpenBaoClusterFinalizer = "openbao.org/openbaocluster-finalizer"

	// DeletionPolicyRetain keeps StatefulSets, PVCs, and external backups.
	DeletionPolicyRetain DeletionPolicy = "Retain"
	// DeletionPolicyDeletePVCs deletes StatefulSets and PVCs, but retains external backups.
	DeletionPolicyDeletePVCs DeletionPolicy = "DeletePVCs"
	// DeletionPolicyDeleteAll deletes StatefulSets, PVCs, and attempts to delete external backups.
	DeletionPolicyDeleteAll DeletionPolicy = "DeleteAll"
)

// ClusterPhase is a high-level summary of cluster state.
// +kubebuilder:validation:Enum=Initializing;Running;Upgrading;BackingUp;Failed
type ClusterPhase string

const (
	ClusterPhaseInitializing ClusterPhase = "Initializing"
	ClusterPhaseRunning      ClusterPhase = "Running"
	ClusterPhaseUpgrading    ClusterPhase = "Upgrading"
	ClusterPhaseBackingUp    ClusterPhase = "BackingUp"
	ClusterPhaseFailed       ClusterPhase = "Failed"
)

// ConditionType identifies a specific aspect of cluster health or lifecycle.
// This type is kept as a strong string alias to avoid stringly-typed code.
type ConditionType string

const (
	// ConditionAvailable indicates whether the cluster is generally available.
	ConditionAvailable ConditionType = "Available"
	// ConditionTLSReady indicates whether TLS assets have been successfully provisioned.
	ConditionTLSReady ConditionType = "TLSReady"
	// ConditionACMEIntegrationReady indicates whether operator-known ACME integration
	// prerequisites are satisfied, such as Gateway passthrough, private ACME trust,
	// and self-reachability checks for supported topologies.
	ConditionACMEIntegrationReady ConditionType = "ACMEIntegrationReady"
	// ConditionACMECacheReady indicates whether the shared ACME cache PVC is ready for use
	// when the configured topology requires or uses a shared ACME cache.
	ConditionACMECacheReady ConditionType = "ACMECacheReady"
	// ConditionAuditFileStorageReady indicates whether the shared audit file storage PVC
	// is ready for file audit devices and mounted by the workload StatefulSets.
	ConditionAuditFileStorageReady ConditionType = "AuditFileStorageReady"
	// ConditionGatewayIntegrationReady indicates whether the operator can verify
	// the referenced Gateway and GatewayClass integration contract for the chosen
	// Gateway API mode.
	ConditionGatewayIntegrationReady ConditionType = "GatewayIntegrationReady"
	// ConditionIngressIntegrationReady indicates whether the operator can verify
	// the managed Ingress integration contract for the chosen ingress mode.
	ConditionIngressIntegrationReady ConditionType = "IngressIntegrationReady"
	// ConditionAPIServerNetworkReady indicates whether the operator can validate
	// the Kubernetes API egress contract used by the operator-managed NetworkPolicy.
	// Unknown means the common service-VIP path is configured, but some CNIs may
	// still require explicit apiServerEndpointIPs for post-DNAT enforcement.
	ConditionAPIServerNetworkReady ConditionType = "APIServerNetworkReady"
	// ConditionBackupConfigurationReady indicates whether the operator can verify
	// the backup Job's operator-known prerequisites such as auth references,
	// storage credential Secret references, and hardened-profile egress rules.
	ConditionBackupConfigurationReady ConditionType = "BackupConfigurationReady"
	// ConditionCloudUnsealIdentityReady indicates whether the operator can
	// determine and validate the cloud KMS unseal authentication path for the
	// main OpenBao Pods when using AWS KMS, GCP Cloud KMS, Azure Key Vault, or OCI KMS.
	ConditionCloudUnsealIdentityReady ConditionType = "CloudUnsealIdentityReady"
	// ConditionProductionReady indicates whether the cluster configuration is considered
	// production-ready by the operator (security posture, unseal, bootstrap flow).
	ConditionProductionReady ConditionType = "ProductionReady"
	// ConditionUserAccessBootstrap indicates whether the operator could recognize
	// a likely user-facing authentication bootstrap path in self-init requests.
	// This is a best-effort heuristic and does not prove that the configured auth
	// methods, roles, or identities are usable.
	ConditionUserAccessBootstrap ConditionType = "UserAccessBootstrap"
	// ConditionUpgrading indicates whether an upgrade is currently in progress.
	ConditionUpgrading ConditionType = "Upgrading"
	// ConditionBackingUp indicates whether a backup is currently in progress.
	ConditionBackingUp ConditionType = "BackingUp"
	// ConditionDegraded indicates the operator has detected a problem requiring attention.
	ConditionDegraded ConditionType = "Degraded"
	// ConditionEtcdEncryptionWarning indicates that etcd encryption may not be enabled,
	// which could expose sensitive data stored in Kubernetes Secrets.
	ConditionEtcdEncryptionWarning ConditionType = "EtcdEncryptionWarning"
	// ConditionSecurityRisk indicates that the cluster is using a relaxed security
	// posture (Development profile) which may not be suitable for production.
	ConditionSecurityRisk ConditionType = "SecurityRisk"
	// ConditionOpenBaoInitialized reflects OpenBao's own initialization state as
	// observed via Kubernetes service registration labels on Pods.
	ConditionOpenBaoInitialized ConditionType = "OpenBaoInitialized"
	// ConditionOpenBaoSealed reflects OpenBao's seal state as observed via
	// Kubernetes service registration labels on Pods.
	ConditionOpenBaoSealed ConditionType = "OpenBaoSealed"
	// ConditionOpenBaoLeader reflects whether a leader could be identified via
	// Kubernetes service registration labels on Pods.
	ConditionOpenBaoLeader ConditionType = "OpenBaoLeader"
	// ConditionNodeSecurityCapabilityMismatch indicates the cluster is configured
	// with workload hardening options that are not supported by the underlying
	// node/Kubernetes environment (for example, AppArmor not available).
	ConditionNodeSecurityCapabilityMismatch ConditionType = "NodeSecurityCapabilityMismatch"
	// ConditionStorageConfigured indicates persistent storage has either been
	// explicitly configured or consistently resolved from observed PVCs.
	ConditionStorageConfigured ConditionType = "StorageConfigured"
	// ConditionReadReplicasReady indicates whether the read-replica pool has the
	// desired number of Ready Pods.
	ConditionReadReplicasReady ConditionType = "ReadReplicasReady"
	// ConditionReadServingAvailable indicates whether the read-replica pool is
	// currently observed in a state that should serve reads for the validated
	// OpenBao version.
	ConditionReadServingAvailable ConditionType = "ReadServingAvailable"
	// ConditionRaftMembershipReady indicates whether observed voter and
	// non-voter membership matches the operator's declared topology.
	ConditionRaftMembershipReady ConditionType = "RaftMembershipReady"
	// ConditionReadReplicasAutopilotHealthy indicates whether the read-replica
	// pool is healthy according to the Raft Autopilot state endpoint.
	ConditionReadReplicasAutopilotHealthy ConditionType = "ReadReplicasAutopilotHealthy"
	// ConditionReadReplicaStorageConfigured indicates whether the read-replica
	// pool storage contract has been explicitly configured or consistently
	// resolved from observed PVCs.
	ConditionReadReplicaStorageConfigured ConditionType = "ReadReplicaStorageConfigured"
)

// TLSMode controls who manages the certificate lifecycle.
// +kubebuilder:validation:Enum=OperatorManaged;External;ACME
type TLSMode string

const (
	// TLSModeOperatorManaged: The operator acts as the CA, generating keys and rotating certs (Current Behavior).
	TLSModeOperatorManaged TLSMode = "OperatorManaged"
	// TLSModeExternal: The operator assumes Secrets are managed by an external entity (cert-manager, user, or CSI driver).
	// The operator will mount them but NOT modify/rotate them.
	TLSModeExternal TLSMode = "External"
	// TLSModeACME: OpenBao uses its native ACME client to fetch certificates.
	// No Secrets are mounted. No sidecar is injected. Best for Zero Trust.
	TLSModeACME TLSMode = "ACME"
)

// Profile defines the security posture for an OpenBaoCluster.
// +kubebuilder:validation:Enum=Hardened;Development
type Profile string

const (
	// ProfileHardened enforces strict security requirements (production-ready).
	ProfileHardened Profile = "Hardened"
	// ProfileDevelopment allows relaxed security for development/testing.
	ProfileDevelopment Profile = "Development"
)

// ACMEConfig configures ACME certificate management for OpenBao.
// See: https://openbao.org/docs/configuration/listener/tcp/#acme-parameters
// +kubebuilder:validation:XValidation:rule="!(has(self.domain) && has(self.domains) && size(self.domains) > 0)",message="tls.acme.domain and tls.acme.domains are mutually exclusive; use only one"
type ACMEConfig struct {
	// DirectoryURL is the ACME directory URL (e.g., "https://acme-v02.api.letsencrypt.org/directory").
	// +kubebuilder:validation:MinLength=1
	DirectoryURL string `json:"directoryURL"`
	// Domain is the domain name for which to obtain the certificate.
	// Deprecated: use Domains to request a certificate with multiple SANs.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Domain string `json:"domain,omitempty"`
	// Domains is the list of domain names for which to obtain the certificate.
	// This maps to OpenBao's listener `tls_acme_domains` field.
	//
	// When empty, the operator will default to an internal Service name suitable for
	// private ACME CAs running inside the cluster (e.g., "<cluster>-acme.<namespace>.svc").
	// +kubebuilder:validation:MinItems=1
	// +optional
	Domains []string `json:"domains,omitempty"`
	// Email is the email address to use for ACME registration.
	// +optional
	Email string `json:"email,omitempty"`
	// SharedCache configures a filesystem cache shared across OpenBao replicas for ACME account
	// and certificate state. This is required for HA ACME topologies where more than one Pod
	// can serve the same hostname concurrently.
	// +optional
	SharedCache *ACMESharedCacheConfig `json:"sharedCache,omitempty"`
}

// ACMESharedCacheMode controls how the operator provides a shared filesystem for OpenBao's ACME cache.
// +kubebuilder:validation:Enum=ManagedPVC;ExistingPVC
type ACMESharedCacheMode string

const (
	// ACMESharedCacheModeManagedPVC instructs the operator to create a dedicated RWX PVC.
	ACMESharedCacheModeManagedPVC ACMESharedCacheMode = "ManagedPVC"
	// ACMESharedCacheModeExistingPVC instructs the operator to mount an existing RWX PVC.
	ACMESharedCacheModeExistingPVC ACMESharedCacheMode = "ExistingPVC"
)

// ACMESharedCacheConfig configures the shared filesystem cache for ACME account and certificate state.
// See: https://openbao.org/docs/configuration/listener/tcp/#acme-parameters
// +kubebuilder:validation:XValidation:rule="self.mode != 'ManagedPVC' || !has(self.existingClaimName) || size(self.existingClaimName) == 0",message="tls.acme.sharedCache.existingClaimName is only supported when mode is ExistingPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ExistingPVC' || size(self.existingClaimName) > 0",message="tls.acme.sharedCache.existingClaimName is required when mode is ExistingPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ExistingPVC' || !has(self.size) || size(self.size) == 0",message="tls.acme.sharedCache.size is only supported when mode is ManagedPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ExistingPVC' || !has(self.storageClassName) || size(self.storageClassName) == 0",message="tls.acme.sharedCache.storageClassName is only supported when mode is ManagedPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ManagedPVC' || size(self.size) > 0",message="tls.acme.sharedCache.size is required when mode is ManagedPVC"
type ACMESharedCacheConfig struct {
	// Mode selects whether the operator creates a dedicated RWX PVC or mounts an existing one.
	Mode ACMESharedCacheMode `json:"mode"`
	// ExistingClaimName is the name of a pre-created RWX PVC in the same namespace.
	// Required when Mode is ExistingPVC.
	// +kubebuilder:validation:MinLength=1
	// +optional
	ExistingClaimName string `json:"existingClaimName,omitempty"`
	// Size is the requested capacity for the managed ACME cache PVC.
	// Required when Mode is ManagedPVC.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Size string `json:"size,omitempty"`
	// StorageClassName is an optional StorageClass for the managed ACME cache PVC.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// AuditFileStorageMode controls how the operator provides shared filesystem storage for file audit logs.
// +kubebuilder:validation:Enum=ManagedPVC;ExistingPVC
type AuditFileStorageMode string

const (
	// AuditFileStorageModeManagedPVC instructs the operator to create a dedicated RWX PVC.
	AuditFileStorageModeManagedPVC AuditFileStorageMode = "ManagedPVC"
	// AuditFileStorageModeExistingPVC instructs the operator to mount an existing RWX PVC.
	AuditFileStorageModeExistingPVC AuditFileStorageMode = "ExistingPVC"
)

// AuditFileStorageConfig configures the shared filesystem integration point for file audit devices.
//
// The operator mounts the selected PVC into each OpenBao Pod. Each Pod uses a
// pod-specific subPath under the same PVC so all Pods can render the same audit
// file path while collectors can mount the PVC read-only and read per-Pod audit
// files from the backing directories. This storage is intended as a collector
// handoff and replay buffer, not as the authoritative compliance archive.
// +kubebuilder:validation:XValidation:rule="self.mode != 'ManagedPVC' || !has(self.existingClaimName) || size(self.existingClaimName) == 0",message="auditFileStorage.existingClaimName is only supported when mode is ExistingPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ExistingPVC' || size(self.existingClaimName) > 0",message="auditFileStorage.existingClaimName is required when mode is ExistingPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ExistingPVC' || !has(self.size) || size(self.size) == 0",message="auditFileStorage.size is only supported when mode is ManagedPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ExistingPVC' || !has(self.storageClassName) || size(self.storageClassName) == 0",message="auditFileStorage.storageClassName is only supported when mode is ManagedPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ManagedPVC' || size(self.size) > 0",message="auditFileStorage.size is required when mode is ManagedPVC"
// +kubebuilder:validation:XValidation:rule="!has(self.mountPath) || (self.mountPath.startsWith('/') && self.mountPath != '/')",message="auditFileStorage.mountPath must be an absolute path and must not be /"
type AuditFileStorageConfig struct {
	// Mode selects whether the operator creates a dedicated RWX PVC or mounts an existing one.
	Mode AuditFileStorageMode `json:"mode"`
	// ExistingClaimName is the name of a pre-created RWX PVC in the same namespace.
	// Required when Mode is ExistingPVC.
	// +kubebuilder:validation:MinLength=1
	// +optional
	ExistingClaimName string `json:"existingClaimName,omitempty"`
	// Size is the requested capacity for the managed audit file storage PVC.
	// Required when Mode is ManagedPVC.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Size string `json:"size,omitempty"`
	// StorageClassName is an optional StorageClass for the managed audit file storage PVC.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
	// MountPath is where the audit file storage PVC is mounted in OpenBao Pods.
	// File audit device paths must be under this path when auditFileStorage is configured.
	// +kubebuilder:default=/openbao/audit
	// +optional
	MountPath string `json:"mountPath,omitempty"`
}

// TLSConfig captures TLS configuration for an OpenBaoCluster.
type TLSConfig struct {
	// Enabled controls whether TLS is enabled for the cluster.
	// +kubebuilder:validation:Required
	Enabled bool `json:"enabled"`
	// Mode controls who manages the certificate lifecycle.
	// +kubebuilder:validation:Enum=OperatorManaged;External;ACME
	// +kubebuilder:default=OperatorManaged
	// +optional
	Mode TLSMode `json:"mode,omitempty"`
	// ACME configures settings when Mode is 'ACME'.
	// +optional
	ACME *ACMEConfig `json:"acme,omitempty"`
	// RotationPeriod is a duration string (for example, "720h") controlling certificate rotation.
	// Only used when Mode is OperatorManaged.
	// +kubebuilder:validation:MinLength=1
	// +optional
	RotationPeriod string `json:"rotationPeriod,omitempty"`
	// ExtraSANs lists additional subject alternative names to include in server certificates.
	// Only used when Mode is OperatorManaged.
	// +optional
	ExtraSANs []string `json:"extraSANs,omitempty"`
}

// StorageConfig captures storage-related configuration for the StatefulSet.
type StorageConfig struct {
	// Size is the requested persistent volume size, for example "10Gi".
	// +kubebuilder:validation:MinLength=1
	Size string `json:"size"`
	// StorageClassName is an optional StorageClass for the PVCs.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// ServiceConfig controls how the main OpenBao Service is exposed.
type ServiceConfig struct {
	// Type is the Kubernetes Service type, for example "ClusterIP" or "LoadBalancer".
	// +optional
	Type corev1.ServiceType `json:"type,omitempty"`
	// Annotations are additional annotations to apply to the Service.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ReadReplicaServiceConfig controls the optional read-only Service for the
// read-replica pool.
type ReadReplicaServiceConfig struct {
	// Enabled controls whether the operator creates a dedicated Service for the
	// read-replica pool.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Type is the Kubernetes Service type, for example "ClusterIP" or
	// "LoadBalancer".
	// +optional
	Type corev1.ServiceType `json:"type,omitempty"`
	// Annotations are additional annotations to apply to the read Service.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ReadReplicaSchedulingConfig defines scheduling overrides for read replicas.
type ReadReplicaSchedulingConfig struct {
	// NodeSelector defines node-selection constraints for read-replica Pods.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations defines Pod tolerations for read-replica Pods.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Affinity defines Pod affinity / anti-affinity rules for read-replica Pods.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// TopologySpreadConstraints defines topology spread constraints for
	// read-replica Pods.
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// ReadReplicaTemplateConfig defines Pod-template overrides for read replicas.
type ReadReplicaTemplateConfig struct {
	// Metadata defines additional labels and annotations applied only to the
	// read-replica Pod template.
	// +optional
	Metadata *PodMetadataConfig `json:"metadata,omitempty"`
	// Resources defines container resource requests and limits for read replicas.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Scheduling defines node-placement and topology overrides for read replicas.
	// +optional
	Scheduling *ReadReplicaSchedulingConfig `json:"scheduling,omitempty"`
}

// ReadReplicaStorageConfig defines storage overrides for the read-replica
// StatefulSet.
type ReadReplicaStorageConfig struct {
	// Size is the requested persistent volume size for read replicas.
	// +optional
	Size *resource.Quantity `json:"size,omitempty"`
	// StorageClassName is an optional StorageClass for read-replica PVCs.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// ReadReplicaConfig defines the steady-state read-replica pool.
type ReadReplicaConfig struct {
	// Replicas is the desired number of permanent non-voters.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
	// Service configures an optional dedicated Service for read traffic.
	// +optional
	Service *ReadReplicaServiceConfig `json:"service,omitempty"`
	// Template configures read-replica-specific Pod template overrides.
	// +optional
	Template *ReadReplicaTemplateConfig `json:"template,omitempty"`
	// Storage configures read-replica-specific storage overrides.
	// +optional
	Storage *ReadReplicaStorageConfig `json:"storage,omitempty"`
}

// IngressPathType identifies how a Kubernetes Ingress path should match requests.
// +kubebuilder:validation:Enum=Prefix;Exact;ImplementationSpecific
type IngressPathType string

const (
	// IngressPathTypePrefix uses prefix path matching.
	IngressPathTypePrefix IngressPathType = "Prefix"
	// IngressPathTypeExact uses exact path matching.
	IngressPathTypeExact IngressPathType = "Exact"
	// IngressPathTypeImplementationSpecific defers path matching to the controller.
	IngressPathTypeImplementationSpecific IngressPathType = "ImplementationSpecific"
)

// IngressReadinessMode identifies how the operator decides whether ingress
// integration is ready for endpoint publication.
// +kubebuilder:validation:Enum=Created;LoadBalancerPublished
type IngressReadinessMode string

const (
	// IngressReadinessModeCreated considers ingress integration ready once the
	// managed Ingress object exists.
	IngressReadinessModeCreated IngressReadinessMode = "Created"
	// IngressReadinessModeLoadBalancerPublished considers ingress integration
	// ready only after the managed Ingress reports a published load balancer
	// address in status.
	IngressReadinessModeLoadBalancerPublished IngressReadinessMode = "LoadBalancerPublished"
)

// IngressConfig controls optional HTTP(S) ingress in front of the OpenBao Service.
type IngressConfig struct {
	// Enabled controls whether the Operator manages an Ingress for external access.
	// +optional
	Enabled bool `json:"enabled"`
	// ClassName is an optional IngressClassName (for example, "nginx", "traefik").
	// +optional
	ClassName *string `json:"className,omitempty"`
	// Host is the primary host for external access, for example "bao.example.com".
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`
	// Path is the HTTP path to route to OpenBao, defaulting to "/".
	// +optional
	Path string `json:"path,omitempty"`
	// PathType identifies how the ingress controller should interpret Path.
	// +kubebuilder:default=Prefix
	// +optional
	PathType IngressPathType `json:"pathType,omitempty"`
	// TLSSecretName is an optional TLS Secret name; when empty the cluster TLS Secret is used.
	// +optional
	TLSSecretName string `json:"tlsSecretName,omitempty"`
	// Annotations are additional annotations to apply to the Ingress.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// ReadinessMode identifies when the operator should consider ingress
	// integration ready for endpoint publication.
	// +kubebuilder:default=LoadBalancerPublished
	// +optional
	ReadinessMode IngressReadinessMode `json:"readinessMode,omitempty"`
}

// MaintenanceConfig defines supported maintenance operations.
// This is intended to provide a first-class workflow for day-2 operations in
// clusters that enforce managed-resource mutation locks via admission policy.
type MaintenanceConfig struct {
	// Enabled enables maintenance mode for this cluster.
	// When true, the operator annotates managed resources (Pods/StatefulSet) with
	// `openbao.org/maintenance=true` to allow controlled restarts/deletes where
	// admission policies require an explicit maintenance signal.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// RestartAt triggers a rolling restart when changed.
	// The operator propagates this value as a Pod template annotation; any change
	// results in a new StatefulSet revision and a controlled restart.
	// Recommended value is an RFC3339 timestamp string.
	// Deprecated: use spec.runtime.restartAt instead. spec.runtime.restartAt
	// takes precedence when both fields are set.
	// +kubebuilder:validation:MinLength=1
	// +optional
	RestartAt string `json:"restartAt,omitempty"`
}

// RuntimeConfig defines explicit runtime control requests for the OpenBao
// workload.
type RuntimeConfig struct {
	// RestartAt triggers a rolling restart when changed.
	// The operator propagates this value as a Pod template annotation; any change
	// results in a new StatefulSet revision and a controlled restart.
	// Recommended value is an RFC3339 timestamp string.
	// +kubebuilder:validation:MinLength=1
	// +optional
	RestartAt string `json:"restartAt,omitempty"`
}

// BackupSchedule defines when and where snapshots are stored.
type BackupSchedule struct {
	// Schedule is a cron-style schedule, for example "0 3 * * *".
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`
	// Target is the object storage configuration for backups.
	Target BackupTarget `json:"target"`
	// JWTAuthRole is the name of the JWT Auth role configured in OpenBao
	// for backup operations. When set, the backup executor will use JWT Auth
	// (projected ServiceAccount token) instead of a static token. This is the preferred authentication
	// method as tokens are automatically rotated by Kubernetes.
	//
	// The role must be configured in OpenBao and must grant the "read" capability on
	// sys/storage/raft/snapshot. The role must bind to the backup ServiceAccount
	// (<cluster-name>-backup-serviceaccount) in the cluster namespace.
	//
	// If OIDC is enabled in SelfInit and this field is empty, a default role
	// named "openbao-operator-backup" will be assumed/created.
	// +optional
	JWTAuthRole string `json:"jwtAuthRole,omitempty"`
	// TokenSecretRef optionally references a Secret containing an OpenBao API
	// token to use for backup operations (fallback method).
	//
	// The Secret must exist in the same namespace as the OpenBaoCluster.
	// Cross-namespace references are not allowed for security reasons.
	//
	// For standard clusters (non self-init), this is typically omitted and the
	// operator uses the root token from <cluster>-root-token. For self-init
	// clusters (no root token Secret), this field must reference a token with
	// permission to read sys/storage/raft/snapshot.
	//
	// If JWTAuthRole is set, this field is ignored in favor of JWT Auth.
	// +optional
	TokenSecretRef *corev1.LocalObjectReference `json:"tokenSecretRef,omitempty"`
	// Retention defines optional backup retention policy.
	// +optional
	Retention *BackupRetention `json:"retention,omitempty"`
	// Image is the container image to use for backup operations.
	// If not specified, defaults to "<repo>:X.Y.Z" where <repo> is derived from OPERATOR_BACKUP_IMAGE_REPOSITORY
	// (default: "ghcr.io/dc-tec/openbao-backup") and the tag matches OPERATOR_VERSION.
	// This allows users to override the image for air-gapped environments or custom registries.
	// +optional
	Image string `json:"image,omitempty"`
}

// UpdateStrategyType defines the type of update strategy to use.
// +kubebuilder:validation:Enum=RollingUpdate;BlueGreen
type UpdateStrategyType string

const (
	// UpdateStrategyRollingUpdate uses a rolling update strategy (default).
	UpdateStrategyRollingUpdate UpdateStrategyType = "RollingUpdate"
	// UpdateStrategyBlueGreen uses a blue/green deployment strategy.
	UpdateStrategyBlueGreen UpdateStrategyType = "BlueGreen"
)

// VerificationConfig allows defining custom health checks before promotion.
type VerificationConfig struct {
	// MinSyncDuration ensures the Green cluster stays healthy as a non-voter
	// for at least this duration before promotion (e.g., "5m").
	// +optional
	MinSyncDuration string `json:"minSyncDuration,omitempty"`

	// PrePromotionHook specifies a Job template to run before promoting Green.
	// The job must complete successfully (exit 0) for promotion to proceed.
	// If the job fails, the operator either aborts or rolls back automatically
	// when blueGreen.autoRollback.onValidationFailure is enabled; otherwise it
	// holds for manual resolution.
	// +optional
	PrePromotionHook *ValidationHookConfig `json:"prePromotionHook,omitempty"`
}

// ValidationHookConfig defines a user-supplied validation Job.
type ValidationHookConfig struct {
	// Image is the container image for the validation job.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Command is the command to run.
	// +optional
	Command []string `json:"command,omitempty"`
	// Args are arguments passed to the command.
	// +optional
	Args []string `json:"args,omitempty"`
	// TimeoutSeconds is the job timeout (default: 300s).
	// +kubebuilder:default=300
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// AutoRollbackConfig defines conditions that trigger automatic rollback.
type AutoRollbackConfig struct {
	// Enabled controls whether automatic rollback is active.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`
	// OnJobFailure triggers rollback when job failures exceed MaxJobFailures.
	// Only applies during early phases (before demoting Blue).
	// +kubebuilder:default=true
	OnJobFailure bool `json:"onJobFailure,omitempty"`
	// OnValidationFailure triggers automatic abort/rollback if the pre-promotion
	// hook fails.
	// +kubebuilder:default=true
	OnValidationFailure bool `json:"onValidationFailure,omitempty"`
}

// BlueGreenConfig configures the behavior when Type is BlueGreen.
type BlueGreenConfig struct {
	// AutoPromote controls whether newly started blue/green upgrades
	// automatically switch traffic and delete the old cluster after sync.
	// If false when an upgrade starts, that upgrade stays in the Syncing
	// phase waiting for an explicit promotion request via spec.upgrade.requests.promote.
	// Changing this field while an upgrade is already in progress affects only
	// future upgrades.
	// +kubebuilder:default=true
	AutoPromote bool `json:"autoPromote"`

	// VerificationConfig allows defining custom health checks before promotion.
	// +optional
	Verification *VerificationConfig `json:"verification,omitempty"`

	// MaxJobFailures is the maximum consecutive job failures before aborting/rolling back.
	// Defaults to 5 if not specified.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxJobFailures *int32 `json:"maxJobFailures,omitempty"`

	// PreUpgradeSnapshot triggers a backup at the start of an upgrade.
	// Creates a recovery point before any changes are made.
	// Requires spec.backup to be configured.
	// +optional
	PreUpgradeSnapshot bool `json:"preUpgradeSnapshot,omitempty"`

	// AutoRollback configures automatic rollback behavior.
	// +optional
	AutoRollback *AutoRollbackConfig `json:"autoRollback,omitempty"`
}

// UpgradeRequestConfig defines one-shot operator requests for upgrade workflows.
type UpgradeRequestConfig struct {
	// Retry requests a retry of the current failed rolling upgrade when changed
	// to a new non-empty value.
	//
	// The operator compares this value against status.upgradeRequests.lastHandledRetry
	// and acts only when the value changes. Recommended value is an RFC3339
	// timestamp string.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Retry string `json:"retry,omitempty"`
	// Promote requests promotion of a held blue/green upgrade when changed to a
	// new non-empty value while spec.upgrade.blueGreen.autoPromote=false.
	//
	// The operator compares this value against
	// status.upgradeRequests.lastHandledPromote and acts only when the value
	// changes. Recommended value is an RFC3339 timestamp string.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Promote string `json:"promote,omitempty"`
	// Rollback requests a manual abort or rollback of the current blue/green
	// upgrade when changed to a new non-empty value.
	//
	// The operator compares this value against
	// status.upgradeRequests.lastHandledRollback and acts only when the value
	// changes. Recommended value is an RFC3339 timestamp string.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Rollback string `json:"rollback,omitempty"`
}

// UpgradeConfig defines configuration for upgrade operations.
type UpgradeConfig struct {
	// Image is the container image to use for upgrade operations.
	//
	// This image is used by Kubernetes Jobs created during upgrades (for example, blue/green
	// cluster orchestration actions). The executor runs inside the tenant namespace and
	// authenticates to OpenBao using a projected ServiceAccount token (JWT auth).
	//
	// If not specified, defaults to "<repo>:X.Y.Z" where <repo> is derived from OPERATOR_UPGRADE_IMAGE_REPOSITORY
	// (default: "ghcr.io/dc-tec/openbao-upgrade") and the tag matches OPERATOR_VERSION.
	// +optional
	Image string `json:"image,omitempty"`

	// PreUpgradeSnapshot, when true, triggers a backup before any upgrade.
	// When enabled, the upgrade manager will create a backup using the backup
	// configuration (spec.backup.target, spec.backup.image, etc.) and
	// wait for it to complete before proceeding with the upgrade.
	//
	// If the backup fails, the upgrade will be blocked and a Degraded condition
	// will be set with Reason=PreUpgradeBackupFailed.
	//
	// Requires spec.backup to be configured with target, image, and
	// authentication (jwtAuthRole or tokenSecretRef).
	// +optional
	PreUpgradeSnapshot bool `json:"preUpgradeSnapshot,omitempty"`
	// JWTAuthRole is the name of the JWT Auth role configured in OpenBao
	// for upgrade executor Jobs. The executor authenticates with a projected
	// ServiceAccount token from <cluster-name>-upgrade-serviceaccount.
	//
	// The role must be configured in OpenBao and must grant the permissions
	// required by the selected upgrade strategy, including:
	// - "read" capability on sys/health
	// - "sudo" and "update" capability on sys/step-down
	// - "read" capability on sys/storage/raft/autopilot/state
	// - for Blue/Green, raft join/configuration/remove-peer/promote/demote operations
	// The role must bind to the upgrade ServiceAccount (<cluster-name>-upgrade-serviceaccount),
	// which is automatically created by the operator.
	//
	// If OIDC is enabled in SelfInit and this field is empty, a default role
	// named "openbao-operator-upgrade" will be assumed/created.
	//
	// This is the supported authentication mechanism for built-in upgrade orchestration.
	// +optional
	JWTAuthRole string `json:"jwtAuthRole,omitempty"`
	// TokenSecretRef optionally references a Secret containing an OpenBao API
	// token for future non-JWT upgrade authentication flows.
	//
	// Built-in rolling and blue/green upgrade orchestration does not support
	// token-based authentication. Configure spec.upgrade.jwtAuthRole or enable
	// spec.selfInit.oidc.enabled instead.
	// +kubebuilder:validation:XValidation:rule="self == null",message="spec.upgrade.tokenSecretRef is not supported; configure spec.upgrade.jwtAuthRole or enable spec.selfInit.oidc.enabled"
	// +optional
	TokenSecretRef *corev1.LocalObjectReference `json:"tokenSecretRef,omitempty"`

	// Strategy defines the update strategy to use.
	// +kubebuilder:default="RollingUpdate"
	Strategy UpdateStrategyType `json:"strategy,omitempty"`

	// Requests defines explicit one-shot operator requests for the current
	// upgrade workflow. The operator acts only when a request value changes.
	// +optional
	Requests *UpgradeRequestConfig `json:"requests,omitempty"`

	// BlueGreen configures the behavior when Strategy is BlueGreen.
	// +optional
	BlueGreen *BlueGreenConfig `json:"blueGreen,omitempty"`
}

// RestoreConfig defines optional configuration for restore operations.
//
// This is primarily used with self-init JWT bootstrap to pre-create a JWT role
// that can be referenced by OpenBaoRestore resources.
type RestoreConfig struct {
	// JWTAuthRole is the name of the JWT Auth role configured in OpenBao
	// for restore operations. When set, and when spec.selfInit.oidc.enabled is true,
	// the operator bootstraps a restore policy and JWT role bound to the restore ServiceAccount
	// (<cluster-name>-restore-serviceaccount).
	//
	// If OIDC is enabled in SelfInit and this field is empty, a default role
	// named "openbao-operator-restore" will be assumed/created.
	//
	// The role must grant "update" capability on sys/storage/raft/snapshot-force.
	//
	// +optional
	JWTAuthRole string `json:"jwtAuthRole,omitempty"`
}

// BackupRetention defines retention policy for backups.
type BackupRetention struct {
	// MaxCount is the maximum number of backups to retain (0 = unlimited).
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxCount int32 `json:"maxCount,omitempty"`
	// MaxAge is the maximum age of backups to retain, e.g., "168h" for 7 days.
	// Backups older than this are deleted after successful new backup upload.
	// +optional
	MaxAge string `json:"maxAge,omitempty"`
}

// NetworkConfig configures network-related settings for the OpenBaoCluster.
type NetworkConfig struct {
	// APIServerCIDR is an optional CIDR block for the Kubernetes API server.
	// When specified, this value is used instead of auto-detection for NetworkPolicy egress rules.
	// This is useful when you want an explicit allow-list (or when the in-cluster service VIP
	// injected into pods is unavailable/unusable in your environment).
	// Example: "10.43.0.0/16" for service network or "192.168.1.0/24" for control plane nodes.
	// +optional
	APIServerCIDR string `json:"apiServerCIDR,omitempty"`

	// APIServerEndpointIPs is an optional list of Kubernetes API server endpoint IPs.
	// When set, the operator adds least-privilege NetworkPolicy egress rules for these IPs on port 6443.
	// This is required on some CNI implementations where egress enforcement happens on the post-NAT
	// destination (the API server endpoint) rather than the kubernetes Service IP (10.43.0.1:443).
	//
	// The operator does not auto-detect these endpoint IPs because doing so reliably requires broader
	// cluster permissions (list/watch). Configure this field explicitly when needed.
	// Example (k3d): ["192.168.166.2"]
	// +optional
	APIServerEndpointIPs []string `json:"apiServerEndpointIPs,omitempty"`

	// DNSNamespace specifies the namespace where the cluster DNS service resides.
	// Defaults to "kube-system" if not specified.
	// +optional
	// +kubebuilder:default="kube-system"
	DNSNamespace string `json:"dnsNamespace,omitempty"`

	// DNSEndpointIPs is an optional list of DNS resolver endpoint IPs that should be
	// allow-listed directly in the operator-managed NetworkPolicy on TCP/UDP port 53.
	//
	// Use this for clusters that resolve DNS through node-local or host-networked caches
	// instead of pod-backed DNS Services in a namespace. These IP-based rules are additive
	// to the namespace-based allow-list controlled by DNSNamespace.
	//
	// The operator does not auto-detect these endpoint IPs because doing so reliably would
	// require environment-specific node or DNS discovery logic outside the current trust model.
	// Example: ["169.254.20.10"]
	// +optional
	DNSEndpointIPs []string `json:"dnsEndpointIPs,omitempty"`

	// EgressRules allows users to specify additional egress rules that will be merged into
	// the operator-managed NetworkPolicy. This is useful for allowing access to external
	// services such as transit seal backends, object storage endpoints, or other dependencies.
	//
	// The operator's default egress rules (DNS, API server, cluster pods) are always included
	// and cannot be overridden. User-provided rules are appended to the operator-managed rules.
	//
	// Example: Allow egress to a transit seal backend in another namespace:
	//   egressRules:
	//   - to:
	//     - namespaceSelector:
	//         matchLabels:
	//           kubernetes.io/metadata.name: transit-namespace
	//     ports:
	//     - protocol: TCP
	//       port: 8200
	// +optional
	EgressRules []networkingv1.NetworkPolicyEgressRule `json:"egressRules,omitempty"`

	// IngressRules allows users to specify additional ingress rules that will be merged into
	// the operator-managed NetworkPolicy. This is useful for allowing access from external
	// services, monitoring tools, or other components that need to reach OpenBao pods.
	//
	// The operator's default ingress rules (cluster pods, kube-system, operator, gateway)
	// are always included and cannot be overridden. User-provided rules are appended to
	// the operator-managed rules.
	//
	// Example: Allow ingress from a monitoring namespace:
	//   ingressRules:
	//   - from:
	//     - namespaceSelector:
	//         matchLabels:
	//           kubernetes.io/metadata.name: monitoring
	//     ports:
	//     - protocol: TCP
	//       port: 8200
	// +optional
	IngressRules []networkingv1.NetworkPolicyIngressRule `json:"ingressRules,omitempty"`

	// TrustedIngressPeers allows users to declare ingress-controller or passthrough-proxy peers
	// that should be allowed to reach OpenBao on the API port without writing full raw
	// NetworkPolicy ingress rules.
	//
	// This is useful for user-managed TCP passthrough or external ingress components that the
	// operator does not manage directly. The operator adds least-privilege ingress rules for
	// port 8200 using these peers.
	//
	// Example: Allow a Traefik namespace to reach OpenBao on port 8200:
	//   trustedIngressPeers:
	//   - namespaceSelector:
	//       matchLabels:
	//         kubernetes.io/metadata.name: traefik
	//
	// Example: Allow only specific ingress-controller pods in another namespace:
	//   trustedIngressPeers:
	//   - namespaceSelector:
	//       matchLabels:
	//         kubernetes.io/metadata.name: ingress-system
	//     podSelector:
	//       matchLabels:
	//         app.kubernetes.io/name: traefik
	// +optional
	TrustedIngressPeers []networkingv1.NetworkPolicyPeer `json:"trustedIngressPeers,omitempty"`
}

// BackupTarget describes a generic, cloud-agnostic object storage destination.
type BackupTarget struct {
	// Provider selects the storage backend. Defaults to "s3" for backward compatibility.
	// +optional
	// +kubebuilder:default=s3
	// +kubebuilder:validation:Enum=s3;gcs;azure
	Provider string `json:"provider,omitempty"`
	// Endpoint is the HTTP(S) endpoint for the object storage service.
	// For S3: Required (e.g., "https://s3.amazonaws.com" or MinIO endpoint).
	// For GCS: Optional (defaults to googleapis.com).
	// For Azure: Optional (derived from StorageAccount if not specified).
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// Bucket is the bucket or container name.
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`
	// PathPrefix is an optional prefix within the bucket for this cluster's snapshots.
	// +optional
	PathPrefix string `json:"pathPrefix,omitempty"`
	// CredentialsSecretRef optionally references a Secret containing credentials for the object store.
	// The Secret must exist in the same namespace as the owning OpenBao resource.
	// Cross-namespace references are not allowed for security reasons.
	//
	// For S3: Expected keys are "accessKeyId" and "secretAccessKey" (optional: "sessionToken", "region", "caCert").
	// For GCS: Expected key is "credentials.json" containing a service account JSON key.
	// For Azure: Expected keys are "accountKey" or "connectionString".
	// Omit this field when relying on ambient workload identity or another default credential chain.
	// +optional
	CredentialsSecretRef *corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
	// WorkloadIdentity optionally applies provider-specific metadata required by cloud workload identity integrations.
	// Use this for ambient identity setups such as EKS Pod Identity or IRSA, GKE Workload Identity, or Azure Workload Identity.
	// When omitted, backup and restore workloads can still use any credentials exposed through the pod's default provider chain.
	// +optional
	WorkloadIdentity *WorkloadIdentityConfig `json:"workloadIdentity,omitempty"`
	// PartSize is the size of each part in multipart uploads (in bytes).
	// Defaults to 10MB (10485760 bytes). Larger values may improve performance for large snapshots
	// on fast networks, while smaller values may be better for slow or unreliable networks.
	// +optional
	// +kubebuilder:default=10485760
	// +kubebuilder:validation:Minimum=5242880
	PartSize int64 `json:"partSize,omitempty"`
	// Concurrency is the number of concurrent parts to upload during multipart uploads.
	// Defaults to 3. Higher values may improve throughput on fast networks but increase
	// memory usage and may overwhelm slower storage backends.
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Concurrency int32 `json:"concurrency,omitempty"`

	// --- S3-specific configuration (only used when Provider=s3) ---

	// Region is the AWS region to use for S3-compatible clients.
	// For AWS, this should match the bucket region (for example, "eu-west-1").
	// For many S3-compatible stores (MinIO/Ceph), this can be any non-empty value.
	// Only used when Provider is "s3".
	// +optional
	// +kubebuilder:default=us-east-1
	Region string `json:"region,omitempty"`
	// RoleARN is the IAM role ARN (or S3-compatible equivalent) to assume via Web Identity.
	// When set, backup and restore Jobs mount a projected ServiceAccount token and set the
	// AWS Web Identity environment variables explicitly.
	// Leave this empty when relying on ambient workload identity or provider-managed default credentials instead.
	// Only used when Provider is "s3".
	// +optional
	RoleARN string `json:"roleArn,omitempty"`
	// UsePathStyle controls whether to use path-style addressing (bucket.s3.amazonaws.com/object)
	// or virtual-hosted-style addressing (bucket.s3.amazonaws.com/object).
	// Set to true for MinIO and S3-compatible stores that require path-style.
	// Set to false for AWS S3 (default, as AWS is deprecating path-style).
	// Only used when Provider is "s3".
	// +optional
	// +kubebuilder:default=false
	UsePathStyle bool `json:"usePathStyle,omitempty"`

	// --- GCS-specific configuration (only used when Provider=gcs) ---

	// GCS contains Google Cloud Storage specific configuration.
	// Only used when Provider is "gcs".
	// +optional
	GCS *GCSTargetConfig `json:"gcs,omitempty"`

	// Azure contains Azure Blob Storage specific configuration.
	// Only used when Provider is "azure".
	// +optional
	Azure *AzureTargetConfig `json:"azure,omitempty"`

	// InsecureSkipVerify allows skipping TLS verification (useful for MinIO/LocalStack/Azurite with self-signed certs).
	// This applies to all providers that support TLS.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// WorkloadIdentityConfig configures cloud workload identity metadata for backup and restore workloads.
type WorkloadIdentityConfig struct {
	// ServiceAccountAnnotations are merged into the generated backup or restore ServiceAccount.
	// This is typically used for provider-specific bindings such as GKE Workload Identity
	// or webhook-based AWS/Azure workload identity integrations.
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`
	// PodLabels are merged into the generated backup or restore Job pod template.
	// This is typically used for provider-specific selectors such as Azure Workload Identity.
	// Operator-managed labels take precedence if the same key is specified here.
	// +optional
	PodLabels map[string]string `json:"podLabels,omitempty"`
}

// GCSTargetConfig holds Google Cloud Storage specific configuration.
type GCSTargetConfig struct {
	// Project is the GCP project ID. Optional if using ADC with default project or
	// if the credentials JSON includes the project.
	// +optional
	Project string `json:"project,omitempty"`
}

// AzureTargetConfig holds Azure Blob Storage specific configuration.
type AzureTargetConfig struct {
	// StorageAccount is the Azure storage account name.
	// Required when using Azure provider.
	// +kubebuilder:validation:MinLength=1
	StorageAccount string `json:"storageAccount,omitempty"`
	// Container is the blob container name. If empty, uses the Bucket field value.
	// +optional
	Container string `json:"container,omitempty"`
}

// SelfInitOperation defines valid operations for self-initialization requests.
// +kubebuilder:validation:Enum=create;read;update;delete;list;patch
type SelfInitOperation string

const (
	// SelfInitOperationCreate creates a new resource.
	SelfInitOperationCreate SelfInitOperation = "create"
	// SelfInitOperationRead reads an existing resource.
	SelfInitOperationRead SelfInitOperation = "read"
	// SelfInitOperationUpdate updates an existing resource.
	SelfInitOperationUpdate SelfInitOperation = "update"
	// SelfInitOperationPatch performs a partial update to an existing resource.
	SelfInitOperationPatch SelfInitOperation = "patch"
	// SelfInitOperationDelete deletes an existing resource.
	SelfInitOperationDelete SelfInitOperation = "delete"
	// SelfInitOperationList lists resources.
	SelfInitOperationList SelfInitOperation = "list"
)

// SelfInitConfig enables OpenBao's self-initialization feature.
// When enabled, OpenBao initializes itself on first start using the configured
// requests, and the root token is automatically revoked.
// See: https://openbao.org/docs/configuration/self-init/
type SelfInitConfig struct {
	// Enabled activates OpenBao's self-initialization feature.
	// When true, the Operator injects initialize stanzas into config.hcl
	// and does NOT create a root token Secret (root token is auto-revoked).
	//
	// WARNING: The root token is auto-revoked during initialization. You MUST
	// configure user authentication (e.g., userpass, JWT, Kubernetes auth) via
	// spec.selfInit.requests before enabling this. spec.selfInit.oidc.enabled
	// only provides Operator authentication for lifecycle tasks, NOT user access.
	// Enabling without user authentication results in permanent lockout.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`
	// OIDC configures JWT authentication for the Operator to perform cluster
	// lifecycle operations (backups, upgrades, restores). When enabled, this
	// sets up the jwt-operator auth method, OIDC discovery, and operator roles.
	// This is for Operator authentication only - users must configure their own
	// authentication methods via spec.selfInit.requests.
	// +optional
	OIDC *SelfInitOIDCConfig `json:"oidc,omitempty"`
	// Requests defines the API operations to execute during self-initialization.
	// Each request becomes a named request block inside an initialize stanza.
	// +optional
	Requests []SelfInitRequest `json:"requests,omitempty"`
}

// SelfInitRequest defines a single API operation to execute during self-initialization.
type SelfInitRequest struct {
	// Name is a unique identifier for this request (used as the block name).
	// Must match regex ^[A-Za-z_][A-Za-z0-9_-]*$
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_-]*$`
	Name string `json:"name"`
	// Operation is the API operation type: create, read, update, delete, or list.
	// +kubebuilder:validation:Enum=create;read;update;delete;list;patch
	Operation SelfInitOperation `json:"operation"`
	// Path is the API path to call (e.g., "sys/audit/stdout", "auth/kubernetes/config").
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// AuditDevice configures an audit device when Path starts with "sys/audit/".
	// This provides structured configuration for audit devices instead of raw JSON.
	// Only used when Path matches the pattern "sys/audit/*".
	// +optional
	AuditDevice *SelfInitAuditDevice `json:"auditDevice,omitempty"`
	// AuthMethod configures an auth method when Path starts with "sys/auth/".
	// This provides structured configuration for enabling auth methods.
	// Only used when Path matches the pattern "sys/auth/*".
	// +optional
	AuthMethod *SelfInitAuthMethod `json:"authMethod,omitempty"`
	// SecretEngine configures a secret engine when Path starts with "sys/mounts/".
	// This provides structured configuration for enabling secret engines.
	// Only used when Path matches the pattern "sys/mounts/*".
	// +optional
	SecretEngine *SelfInitSecretEngine `json:"secretEngine,omitempty"`
	// Policy configures a policy when Path starts with "sys/policies/".
	// This provides structured configuration for creating/updating policies.
	// Only used when Path matches the pattern "sys/policies/*".
	// +optional
	Policy *SelfInitPolicy `json:"policy,omitempty"`
	// Data contains the request payload for paths that don't have structured types.
	// This must be a JSON/YAML object whose shape matches the target API endpoint.
	// Nested maps and lists are supported and are rendered into the initialize stanza as HCL objects.
	//
	// **Note:** For common paths, use structured types instead:
	// - `sys/audit/*` → use `auditDevice`
	// - `sys/auth/*` → use `authMethod`
	// - `sys/mounts/*` → use `secretEngine`
	// - `sys/policies/*` → use `policy`
	//
	// This payload is stored in the OpenBaoCluster resource and persisted in etcd;
	// it must not contain sensitive values such as tokens, passwords, or unseal keys.
	// +optional
	Data *apiextensionsv1.JSON `json:"data,omitempty"`
	// AllowFailure allows this request to fail without blocking initialization.
	// Defaults to false.
	// +optional
	AllowFailure bool `json:"allowFailure,omitempty"`
}

// SelfInitAuditDevice provides structured configuration for enabling audit devices
// via self-init requests. This replaces the need for raw JSON in the Data field.
// See: https://openbao.org/api-docs/system/audit/
// +kubebuilder:validation:XValidation:rule="self.type == 'file' || !has(self.fileOptions)",message="fileOptions is only supported when type is file"
// +kubebuilder:validation:XValidation:rule="self.type != 'file' || has(self.fileOptions)",message="fileOptions is required when type is file"
// +kubebuilder:validation:XValidation:rule="self.type == 'http' || !has(self.httpOptions)",message="httpOptions is only supported when type is http"
// +kubebuilder:validation:XValidation:rule="self.type != 'http' || has(self.httpOptions)",message="httpOptions is required when type is http"
// +kubebuilder:validation:XValidation:rule="self.type == 'syslog' || !has(self.syslogOptions)",message="syslogOptions is only supported when type is syslog"
// +kubebuilder:validation:XValidation:rule="self.type == 'socket' || !has(self.socketOptions)",message="socketOptions is only supported when type is socket"
type SelfInitAuditDevice struct {
	// Type is the type of audit device (e.g., "file", "syslog", "socket", "http").
	// +kubebuilder:validation:Enum=file;syslog;socket;http
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
	// Description is an optional description for the audit device.
	// +optional
	Description string `json:"description,omitempty"`
	// FileOptions configures options for file audit devices.
	// Only used when Type is "file".
	// +optional
	FileOptions *FileAuditOptions `json:"fileOptions,omitempty"`
	// HTTPOptions configures options for HTTP audit devices.
	// Only used when Type is "http".
	// +optional
	HTTPOptions *HTTPAuditOptions `json:"httpOptions,omitempty"`
	// SyslogOptions configures options for syslog audit devices.
	// Only used when Type is "syslog".
	// +optional
	SyslogOptions *SyslogAuditOptions `json:"syslogOptions,omitempty"`
	// SocketOptions configures options for socket audit devices.
	// Only used when Type is "socket".
	// +optional
	SocketOptions *SocketAuditOptions `json:"socketOptions,omitempty"`
}

// SelfInitAuthMethod provides structured configuration for enabling auth methods
// via self-init requests. This replaces the need for raw JSON in the Data field.
// See: https://openbao.org/api-docs/system/auth/
type SelfInitAuthMethod struct {
	// Type is the type of auth method (e.g., "jwt", "kubernetes", "userpass", "ldap").
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
	// Description is an optional description for the auth method.
	// +optional
	Description string `json:"description,omitempty"`
	// Config contains optional configuration for the auth method mount.
	// Common fields include: default_lease_ttl, max_lease_ttl, listing_visibility, etc.
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// SelfInitSecretEngine provides structured configuration for enabling secret engines
// via self-init requests. This replaces the need for raw JSON in the Data field.
// See: https://openbao.org/api-docs/system/mounts/
type SelfInitSecretEngine struct {
	// Type is the type of secret engine (e.g., "kv", "pki", "transit", "database").
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
	// Description is an optional description for the secret engine.
	// +optional
	Description string `json:"description,omitempty"`
	// Options contains optional configuration specific to the secret engine type.
	// For KV engines, common options include: version ("1" or "2").
	// For other engines, options vary by type.
	// +optional
	Options map[string]string `json:"options,omitempty"`
}

// SelfInitPolicy provides structured configuration for creating/updating policies
// via self-init requests. This replaces the need for raw JSON in the Data field.
// See: https://openbao.org/api-docs/system/policies-acl/
type SelfInitPolicy struct {
	// Policy is the HCL or JSON policy content.
	// This is the actual policy rules that will be applied.
	// +kubebuilder:validation:MinLength=1
	Policy string `json:"policy"`
}

// GatewayConfig configures Kubernetes Gateway API access for the OpenBao cluster.
// This is an alternative to Ingress for external access, using the more modern
// and expressive Gateway API.
type GatewayConfig struct {
	// Enabled activates Gateway API support for this cluster.
	// When true, the Operator creates an HTTPRoute for the cluster.
	Enabled bool `json:"enabled"`
	// ListenerName optionally targets a specific listener (sectionName) on the referenced Gateway.
	// When set, the generated Route (HTTPRoute or TLSRoute) attaches only to that listener.
	// This is useful when a Gateway exposes multiple listeners for the same hostname (e.g. Traefik
	// "web" and "websecure") and you want deterministic attachment.
	// +optional
	ListenerName string `json:"listenerName,omitempty"`
	// GatewayRef references an existing Gateway resource that will handle
	// traffic for this OpenBao cluster. The Gateway must already exist.
	GatewayRef GatewayReference `json:"gatewayRef"`
	// Hostname for routing traffic to this OpenBao cluster.
	// This hostname will be automatically added to the TLS SANs.
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname"`
	// Path prefix for the HTTPRoute (defaults to "/").
	// +optional
	Path string `json:"path,omitempty"`
	// Annotations to apply to the HTTPRoute resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// BackendTLS configures BackendTLSPolicy for end-to-end TLS between the Gateway and OpenBao.
	// When enabled, the Operator creates a BackendTLSPolicy that configures the Gateway to use
	// HTTPS when communicating with the OpenBao backend service and validates the backend
	// certificate using the cluster's CA certificate.
	// +optional
	BackendTLS *BackendTLSConfig `json:"backendTLS,omitempty"`
	// TLSPassthrough enables TLS passthrough mode using TLSRoute instead of HTTPRoute.
	// When true, the Operator creates a TLSRoute that routes encrypted TLS traffic based on SNI
	// without terminating TLS at the Gateway. OpenBao terminates TLS directly.
	// When false (default), the Operator creates an HTTPRoute with TLS termination at the Gateway.
	// Note: TLSRoute and HTTPRoute are mutually exclusive - only one can be used per cluster.
	// BackendTLSPolicy is not needed when TLSPassthrough is enabled since the Gateway does not
	// decrypt traffic. The Gateway listener must be configured with protocol: TLS and mode: Passthrough.
	// +optional
	TLSPassthrough bool `json:"tlsPassthrough,omitempty"`
}

// BackendTLSConfig configures BackendTLSPolicy for Gateway API.
type BackendTLSConfig struct {
	// Enabled controls whether the Operator creates a BackendTLSPolicy.
	// When true (default when Gateway is enabled), the Operator creates a BackendTLSPolicy
	// that enables HTTPS and certificate validation for backend connections.
	// When false, no BackendTLSPolicy is created and the Gateway will use HTTP (or rely on
	// external configuration for TLS).
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
	// Hostname is the hostname to verify in the backend certificate.
	// If not specified, defaults to the Service DNS name: <service-name>.<namespace>.svc
	// This should match the certificate SAN or the service DNS name.
	// +optional
	Hostname string `json:"hostname,omitempty"`
}

// InitContainerConfig configures the init container used to render OpenBao configuration.
// The init container is responsible for rendering the final config.hcl from a template
// using environment variables such as HOSTNAME and POD_IP.
//
// The operator relies on this init container to render config.hcl at runtime. Disabling
// the init container is not supported and will be rejected by validation.
type InitContainerConfig struct {
	// Enabled controls whether the init container is used to render the configuration.
	// The operator requires the init container; disabling it is not supported.
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Image is the container image to use for the init container.
	// If not specified, defaults to "<repo>:X.Y.Z" where <repo> is derived from OPERATOR_INIT_IMAGE_REPOSITORY
	// (default: "ghcr.io/dc-tec/openbao-init") and the tag matches OPERATOR_VERSION.
	// +optional
	Image string `json:"image,omitempty"`
}

// GatewayReference identifies a Gateway resource.
type GatewayReference struct {
	// Name of the Gateway resource.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Namespace of the Gateway resource. If empty, uses the OpenBaoCluster namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// TransitSealConfig configures the Transit seal type.
// See: https://openbao.org/docs/configuration/seal/transit/
type TransitSealConfig struct {
	// Address is the full HTTPS address to the OpenBao cluster providing the Transit seal.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`

	// Token is the OpenBao token to use for authentication.
	// Note: It is strongly recommended to use CredentialsSecretRef instead of setting this directly.
	// +optional
	Token string `json:"token,omitempty"`

	// KeyName is the transit key to use for encryption and decryption.
	// +kubebuilder:validation:MinLength=1
	KeyName string `json:"keyName"`

	// MountPath is the mount path to the transit secret engine.
	// +kubebuilder:validation:MinLength=1
	MountPath string `json:"mountPath"`

	// Namespace is the namespace path to the transit secret engine.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// DisableRenewal disables automatic token renewal.
	// Set to true if token lifecycle is managed externally (e.g., by OpenBao Agent).
	// +optional
	DisableRenewal *bool `json:"disableRenewal,omitempty"`

	// TLSCACert is the path to the CA certificate file for TLS communication.
	// +optional
	TLSCACert string `json:"tlsCACert,omitempty"`

	// TLSClientCert is the path to the client certificate for TLS communication.
	// +optional
	TLSClientCert string `json:"tlsClientCert,omitempty"`

	// TLSClientKey is the path to the private key for TLS communication.
	// +optional
	TLSClientKey string `json:"tlsClientKey,omitempty"`

	// TLSServerName is the SNI host name to use when connecting via TLS.
	// +optional
	TLSServerName string `json:"tlsServerName,omitempty"`

	// TLSSkipVerify disables verification of TLS certificates.
	// Using this option is highly discouraged and decreases security.
	// +optional
	TLSSkipVerify *bool `json:"tlsSkipVerify,omitempty"`
}

// AWSKMSSealConfig configures the AWS KMS seal type.
// See: https://openbao.org/docs/configuration/seal/awskms/
type AWSKMSSealConfig struct {
	// Region is the AWS region where the encryption key lives.
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// KMSKeyID is the AWS KMS key ID or ARN to use for encryption and decryption.
	// An alias in the format "alias/key-alias-name" may also be used.
	// +kubebuilder:validation:MinLength=1
	KMSKeyID string `json:"kmsKeyID"`

	// Endpoint is the KMS API endpoint to be used for AWS KMS requests.
	// Useful when connecting to KMS over a VPC Endpoint.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// AccessKey is the AWS access key ID to use.
	// Note: It is strongly recommended to use CredentialsSecretRef or Workload Identity (IRSA) instead.
	// +optional
	AccessKey string `json:"accessKey,omitempty"`

	// SecretKey is the AWS secret access key to use.
	// Note: It is strongly recommended to use CredentialsSecretRef or Workload Identity (IRSA) instead.
	// +optional
	SecretKey string `json:"secretKey,omitempty"`

	// SessionToken specifies the AWS session token.
	// +optional
	SessionToken string `json:"sessionToken,omitempty"`
}

// AzureKeyVaultSealConfig configures the Azure Key Vault seal type.
// See: https://openbao.org/docs/configuration/seal/azurekeyvault/
type AzureKeyVaultSealConfig struct {
	// VaultName is the name of the Azure Key Vault.
	// +kubebuilder:validation:MinLength=1
	VaultName string `json:"vaultName"`

	// KeyName is the name of the key in the Azure Key Vault.
	// +kubebuilder:validation:MinLength=1
	KeyName string `json:"keyName"`

	// TenantID is the Azure tenant ID.
	// +optional
	TenantID string `json:"tenantID,omitempty"`

	// ClientID is the Azure client ID.
	// +optional
	ClientID string `json:"clientID,omitempty"`

	// ClientSecret is the Azure client secret.
	// Note: It is strongly recommended to use CredentialsSecretRef or Managed Service Identity instead.
	// +optional
	ClientSecret string `json:"clientSecret,omitempty"`

	// Resource is the Azure AD resource endpoint.
	// For Managed HSM, this should usually be "managedhsm.azure.net".
	// +optional
	Resource string `json:"resource,omitempty"`

	// Environment is the Azure environment (e.g., "AzurePublicCloud", "AzureUSGovernmentCloud").
	// +optional
	Environment string `json:"environment,omitempty"`
}

// GCPCloudKMSSealConfig configures the GCP Cloud KMS seal type.
// See: https://openbao.org/docs/configuration/seal/gcpckms/
type GCPCloudKMSSealConfig struct {
	// Project is the GCP project ID.
	// +kubebuilder:validation:MinLength=1
	Project string `json:"project"`

	// Region is the GCP region where the key ring lives.
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// KeyRing is the name of the GCP KMS key ring.
	// +kubebuilder:validation:MinLength=1
	KeyRing string `json:"keyRing"`

	// CryptoKey is the name of the GCP KMS crypto key.
	// +kubebuilder:validation:MinLength=1
	CryptoKey string `json:"cryptoKey"`

	// Credentials is the path to the GCP credentials JSON file.
	// Note: It is strongly recommended to use CredentialsSecretRef or Workload Identity instead.
	// +optional
	Credentials string `json:"credentials,omitempty"`
}

// KMIPSealConfig configures the KMIP seal type.
// See: https://openbao.org/docs/configuration/seal/kmip/
type KMIPSealConfig struct {
	// Endpoint is the KMIP server endpoint.
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`

	// KMSKeyID is the unique identifier of the KMIP key to use.
	// +kubebuilder:validation:MinLength=1
	KMSKeyID string `json:"kmsKeyID"`

	// ClientCert is the path to the client certificate used for KMIP communication.
	// +kubebuilder:validation:MinLength=1
	ClientCert string `json:"clientCert"`

	// ClientKey is the path to the private key used for KMIP communication.
	// +kubebuilder:validation:MinLength=1
	ClientKey string `json:"clientKey"`

	// CACert is the path to the CA certificate for KMIP communication.
	// +optional
	CACert string `json:"caCert,omitempty"`

	// ServerName is the TLS server name to use when connecting to the KMIP endpoint.
	// +optional
	ServerName string `json:"serverName,omitempty"`

	// Timeout is the timeout in seconds for KMIP requests.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Timeout *int32 `json:"timeout,omitempty"`

	// EncryptAlg is the encryption algorithm used for KMIP requests.
	// +kubebuilder:validation:Enum=AES_GCM;RSA_OAEP_SHA256;RSA_OAEP_SHA384;RSA_OAEP_SHA512
	// +optional
	EncryptAlg string `json:"encryptAlg,omitempty"`

	// TLS12Ciphers configures the TLS 1.2 cipher suites to use when connecting
	// to the KMIP endpoint.
	// +optional
	TLS12Ciphers string `json:"tls12Ciphers,omitempty"`

	// Disabled disables this seal configuration, for example during seal migration.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`
}

// OCIKMSSealConfig configures the OCI KMS seal type.
// See: https://openbao.org/docs/configuration/seal/ocikms/
type OCIKMSSealConfig struct {
	// KeyID is the OCID of the master encryption key.
	// +kubebuilder:validation:MinLength=1
	KeyID string `json:"keyID"`

	// CryptoEndpoint is the OCI KMS crypto endpoint.
	// +kubebuilder:validation:MinLength=1
	CryptoEndpoint string `json:"cryptoEndpoint"`

	// ManagementEndpoint is the OCI KMS management endpoint.
	// +kubebuilder:validation:MinLength=1
	ManagementEndpoint string `json:"managementEndpoint"`

	// AuthTypeAPIKey enables OCI API key authentication through an OCI SDK config file.
	// When false or omitted, OpenBao uses the default OCI principal flow for the runtime
	// environment, such as instance principal.
	// +optional
	AuthTypeAPIKey *bool `json:"authTypeAPIKey,omitempty"`

	// Disabled disables this seal configuration, for example during seal migration.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`
}

// PKCS11RuntimeConfig configures local runtime wiring needed by PKCS#11 vendor
// libraries. It is intentionally scoped to environment variables and library
// lookup paths so HSM integrations do not require custom wrapper scripts.
type PKCS11RuntimeConfig struct {
	// LibraryPath sets LD_LIBRARY_PATH for the OpenBao process. Use this when
	// the configured PKCS#11 module depends on sibling vendor libraries that
	// are not in the image's default dynamic linker search path.
	// +optional
	LibraryPath string `json:"libraryPath,omitempty"`

	// Env exposes literal environment variables from keys in
	// spec.unseal.credentialsSecretRef. Use this for vendor runtime settings
	// such as HSM endpoints or authentication key references.
	// +kubebuilder:validation:MaxItems=16
	// +optional
	Env []PKCS11RuntimeEnvVar `json:"env,omitempty"`

	// FileEnv exposes environment variables whose values are paths to files
	// mounted from keys in spec.unseal.credentialsSecretRef. Use this for vendor
	// settings that expect a config file path, for example SOFTHSM2_CONF or
	// vendor-specific PKCS#11 client configuration variables.
	// +kubebuilder:validation:MaxItems=16
	// +optional
	FileEnv []PKCS11RuntimeFileEnvVar `json:"fileEnv,omitempty"`
}

// PKCS11RuntimeEnvVar maps a PKCS#11 runtime environment variable to a key in
// spec.unseal.credentialsSecretRef.
type PKCS11RuntimeEnvVar struct {
	// Name is the environment variable name to expose to the OpenBao process.
	// Names owned by OpenBao's PKCS#11 seal configuration, such as BAO_HSM_PIN,
	// are managed by the operator and must not be configured here.
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
	Name string `json:"name"`

	// SecretKey is the key in spec.unseal.credentialsSecretRef to source as the
	// environment variable value.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[-._A-Za-z0-9]+$`
	SecretKey string `json:"secretKey"`
}

// PKCS11RuntimeFileEnvVar maps a PKCS#11 runtime environment variable to the
// mounted file path for a key in spec.unseal.credentialsSecretRef.
type PKCS11RuntimeFileEnvVar struct {
	// Name is the environment variable name to expose to the OpenBao process.
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
	Name string `json:"name"`

	// SecretKey is the key in spec.unseal.credentialsSecretRef whose mounted
	// file path should become the environment variable value.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[-._A-Za-z0-9]+$`
	SecretKey string `json:"secretKey"`
}

// PKCS11SealConfig configures the PKCS#11 seal type.
// See: https://openbao.org/docs/configuration/seal/pkcs11/
// +kubebuilder:validation:XValidation:rule="(has(self.slot) && size(self.slot) > 0) || (has(self.tokenLabel) && size(self.tokenLabel) > 0)",message="spec.unseal.pkcs11.slot or spec.unseal.pkcs11.tokenLabel is required"
// +kubebuilder:validation:XValidation:rule="!(has(self.slot) && size(self.slot) > 0 && has(self.tokenLabel) && size(self.tokenLabel) > 0)",message="spec.unseal.pkcs11.slot and spec.unseal.pkcs11.tokenLabel are mutually exclusive"
type PKCS11SealConfig struct {
	// Lib is the path to the PKCS#11 library provided by the HSM vendor.
	// +kubebuilder:validation:MinLength=1
	Lib string `json:"lib"`

	// Slot is the slot number where the HSM token is located.
	// +optional
	Slot string `json:"slot,omitempty"`

	// TokenLabel is the token label of the HSM slot to use instead of Slot.
	// +optional
	TokenLabel string `json:"tokenLabel,omitempty"`

	// PIN is the PIN for accessing the HSM token.
	// Note: It is strongly recommended to use CredentialsSecretRef instead of setting this directly.
	// +optional
	PIN string `json:"pin,omitempty"`

	// KeyLabel is the label for the encryption key used by OpenBao.
	// +kubebuilder:validation:MinLength=1
	KeyLabel string `json:"keyLabel"`

	// KeyID is the PKCS#11 key identifier to use instead of KeyLabel.
	// +optional
	KeyID string `json:"keyID,omitempty"`

	// Mechanism overrides the PKCS#11 wrapping or encryption mechanism.
	// +optional
	Mechanism string `json:"mechanism,omitempty"`

	// DisableSoftwareEncryption disables the software encryption fallback.
	// +optional
	DisableSoftwareEncryption *bool `json:"disableSoftwareEncryption,omitempty"`

	// Disabled disables this seal configuration, for example during seal migration.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// RSAOAEPHash specifies the hash algorithm to use for RSA with OAEP padding.
	// Valid values: sha1, sha224, sha256, sha384, sha512.
	// +kubebuilder:validation:Enum=sha1;sha224;sha256;sha384;sha512
	// +optional
	RSAOAEPHash string `json:"rsaOAEPHash,omitempty"`

	// Runtime configures local PKCS#11 vendor runtime wiring such as library
	// lookup paths and environment variables sourced from credentialsSecretRef.
	// +optional
	Runtime *PKCS11RuntimeConfig `json:"runtime,omitempty"`
}

// StaticSealConfig configures the static seal type.
// This is the default seal type managed by the operator.
// See: https://openbao.org/docs/configuration/seal/static-key/
type StaticSealConfig struct {
	// CurrentKey is the path to the static unseal key file.
	// Defaults to "file:///etc/bao/unseal/key" (operator-managed).
	// +optional
	CurrentKey string `json:"currentKey,omitempty"`

	// CurrentKeyID is the identifier for the current unseal key.
	// Defaults to "operator-generated-v1" (operator-managed).
	// +optional
	CurrentKeyID string `json:"currentKeyID,omitempty"`
}

// UnsealConfig defines the auto-unseal configuration for an OpenBaoCluster.
// If omitted, defaults to "static" mode managed by the operator.
type UnsealConfig struct {
	// Type specifies the seal type.
	// Defaults to "static".
	// +kubebuilder:validation:Enum=static;awskms;gcpckms;azurekeyvault;transit;kmip;ocikms;pkcs11
	// +kubebuilder:default=static
	Type string `json:"type,omitempty"`

	// Static configures the static seal type.
	// Optional when Type is "static" (operator provides defaults if omitted).
	// +optional
	Static *StaticSealConfig `json:"static,omitempty"`

	// Transit configures the Transit seal type.
	// Required when Type is "transit".
	// +optional
	Transit *TransitSealConfig `json:"transit,omitempty"`

	// AWSKMS configures the AWS KMS seal type.
	// Required when Type is "awskms".
	// +optional
	AWSKMS *AWSKMSSealConfig `json:"awskms,omitempty"`

	// AzureKeyVault configures the Azure Key Vault seal type.
	// Required when Type is "azurekeyvault".
	// +optional
	AzureKeyVault *AzureKeyVaultSealConfig `json:"azureKeyVault,omitempty"`

	// GCPCloudKMS configures the GCP Cloud KMS seal type.
	// Required when Type is "gcpckms".
	// +optional
	GCPCloudKMS *GCPCloudKMSSealConfig `json:"gcpCloudKMS,omitempty"`

	// KMIP configures the KMIP seal type.
	// Required when Type is "kmip".
	// +optional
	KMIP *KMIPSealConfig `json:"kmip,omitempty"`

	// OCIKMS configures the OCI KMS seal type.
	// Required when Type is "ocikms".
	// +optional
	OCIKMS *OCIKMSSealConfig `json:"ocikms,omitempty"`

	// PKCS11 configures the PKCS#11 seal type.
	// Required when Type is "pkcs11".
	// +optional
	PKCS11 *PKCS11SealConfig `json:"pkcs11,omitempty"`

	// CredentialsSecretRef references a Secret containing provider credentials
	// (for example AWS access keys, GCP credentials.json, Azure client-secret keys,
	// or OCI SDK config for authTypeAPIKey mode).
	// If using Workload Identity (IRSA, GKE WI, Azure MSI), this can be omitted.
	// The Secret must exist in the same namespace as the OpenBaoCluster.
	// Cross-namespace references are not allowed for security reasons.
	// +optional
	CredentialsSecretRef *corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
}

// ImageVerificationConfig configures supply chain security checks for container images.
// When enabled, verification applies to all operator-managed images for this cluster (StatefulSets, Deployments, and Jobs).
type ImageVerificationConfig struct {
	// Enabled controls whether image verification is enforced.
	Enabled bool `json:"enabled"`

	// PublicKey is the Cosign public key content used to verify the signature.
	// Required for static key verification. If empty, keyless verification will be used
	// (requires Issuer and Subject to be set).
	// +optional
	PublicKey string `json:"publicKey,omitempty"`

	// Issuer is the OIDC issuer for keyless verification (e.g., https://token.actions.githubusercontent.com).
	// Required for keyless verification when PublicKey is not provided.
	// For GitHub Actions keyless verification, use: https://token.actions.githubusercontent.com
	// +optional
	Issuer string `json:"issuer,omitempty"`

	// Subject is the OIDC subject for keyless verification.
	// Required for keyless verification when PublicKey is not provided.
	// Example (GitHub Actions): https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml@refs/tags/<VERSION>
	// The version in the subject MUST match the image tag version.
	// +optional
	Subject string `json:"subject,omitempty"`

	// IssuerRegExp is a regular expression for the OIDC issuer when using keyless verification.
	// Use this to allow a controlled set of issuers instead of a single exact issuer string.
	// Requires SubjectRegExp when PublicKey is not provided.
	// +optional
	IssuerRegExp string `json:"issuerRegExp,omitempty"`

	// SubjectRegExp is a regular expression for the OIDC subject when using keyless verification.
	// Use this to allow a controlled set of workflow identities instead of a single exact subject.
	// Requires IssuerRegExp when PublicKey is not provided.
	// +optional
	SubjectRegExp string `json:"subjectRegExp,omitempty"`

	// FailurePolicy defines behavior on verification failure.
	// "Block" blocks reconciliation of the affected workload when verification fails.
	// "Warn" logs an error and emits a Kubernetes Event but proceeds.
	// +kubebuilder:validation:Enum=Warn;Block
	// +kubebuilder:default=Block
	FailurePolicy string `json:"failurePolicy"`

	// IgnoreTlog controls whether to verify against the Rekor transparency log.
	// When false (default), signatures are verified against Rekor for non-repudiation.
	// When true, only signature verification is performed without transparency log checks.
	// +optional
	// +kubebuilder:default=false
	IgnoreTlog bool `json:"ignoreTlog,omitempty"`

	// ImagePullSecrets is a list of references to secrets in the same namespace
	// to use for pulling images from private registries during verification.
	// These secrets must be of type kubernetes.io/dockerconfigjson or kubernetes.io/dockercfg.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// WorkloadHardeningConfig configures optional workload hardening features.
type WorkloadHardeningConfig struct {
	// AppArmorEnabled controls whether the operator sets AppArmor profiles on
	// generated Pods and Jobs. Some Kubernetes environments do not support AppArmor;
	// this is opt-in to avoid scheduling failures.
	// +optional
	AppArmorEnabled bool `json:"appArmorEnabled,omitempty"`
}

// OpenBaoConfiguration defines the server configuration for OpenBao.
type OpenBaoConfiguration struct {
	// UI enables the built-in web interface.
	// +kubebuilder:default=true
	// +optional
	UI *bool `json:"ui,omitempty"`

	// LogLevel specifies the log level.
	// +kubebuilder:validation:Enum=trace;debug;info;warn;err
	// +kubebuilder:default=info
	// +optional
	LogLevel string `json:"logLevel,omitempty"`

	// Listener allows tuning the TCP listener.
	// Note: Address and ClusterAddress are managed by the operator and cannot be changed.
	// +optional
	Listener *ListenerConfig `json:"listener,omitempty"`

	// Raft allows tuning the Raft storage backend.
	// +optional
	Raft *RaftConfig `json:"raft,omitempty"`

	// ACMECARoot is the path to the ACME CA root certificate file.
	// This is used when TLS mode is ACME to specify a custom CA root for ACME certificate validation.
	// +optional
	ACMECARoot string `json:"acmeCARoot,omitempty"`

	// Logging allows configuring logging behavior.
	// +optional
	Logging *LoggingConfig `json:"logging,omitempty"`

	// Plugin allows configuring plugin behavior.
	// Note: This is separate from spec.plugins which defines plugin instances.
	// +optional
	Plugin *PluginConfig `json:"plugin,omitempty"`

	// DefaultLeaseTTL is the default lease TTL for tokens and secrets (e.g., "720h", "30m").
	// If not specified, OpenBao uses its default.
	// +optional
	DefaultLeaseTTL string `json:"defaultLeaseTTL,omitempty"`

	// MaxLeaseTTL is the maximum lease TTL for tokens and secrets (e.g., "8760h", "1y").
	// This must be greater than or equal to DefaultLeaseTTL.
	// If not specified, OpenBao uses its default.
	// +optional
	MaxLeaseTTL string `json:"maxLeaseTTL,omitempty"`

	// CacheSize is the size of the cache in bytes.
	// If not specified, OpenBao uses its default cache size.
	// +kubebuilder:validation:Minimum=0
	// +optional
	CacheSize *int64 `json:"cacheSize,omitempty"`

	// DisableCache disables the cache entirely.
	// When true, all caching is disabled.
	// +optional
	DisableCache *bool `json:"disableCache,omitempty"`

	// DetectDeadlocks enables deadlock detection in OpenBao.
	// This is an experimental feature for debugging.
	// +optional
	DetectDeadlocks *bool `json:"detectDeadlocks,omitempty"`

	// RawStorageEndpoint enables the raw storage endpoint.
	// This is an experimental feature that exposes raw storage operations.
	// +optional
	RawStorageEndpoint *bool `json:"rawStorageEndpoint,omitempty"`

	// IntrospectionEndpoint enables the introspection endpoint.
	// This is an experimental feature for debugging and introspection.
	// +optional
	IntrospectionEndpoint *bool `json:"introspectionEndpoint,omitempty"`

	// ImpreciseLeaseRoleTracking enables imprecise lease role tracking.
	// This is an experimental feature that may improve performance in some scenarios.
	// +optional
	ImpreciseLeaseRoleTracking *bool `json:"impreciseLeaseRoleTracking,omitempty"`

	// UnsafeAllowAPIAuditCreation allows API-based audit device creation.
	// This bypasses the normal audit device configuration validation.
	// Use with caution.
	// +optional
	UnsafeAllowAPIAuditCreation *bool `json:"unsafeAllowAPIAuditCreation,omitempty"`

	// AllowAuditLogPrefixing allows audit log prefixing.
	// This enables custom prefixes in audit log entries.
	// +optional
	AllowAuditLogPrefixing *bool `json:"allowAuditLogPrefixing,omitempty"`

	// EnableResponseHeaderHostname enables the hostname in response headers.
	// When true, OpenBao includes the hostname in HTTP response headers.
	// +optional
	EnableResponseHeaderHostname *bool `json:"enableResponseHeaderHostname,omitempty"`

	// EnableResponseHeaderRaftNodeID enables the Raft node ID in response headers.
	// When true, OpenBao includes the Raft node ID in HTTP response headers.
	// +optional
	EnableResponseHeaderRaftNodeID *bool `json:"enableResponseHeaderRaftNodeID,omitempty"`
}

// ListenerConfig allows tuning the TCP listener configuration.
type ListenerConfig struct {
	// TLSDisable controls TLS on the listener.
	// Note: This is typically managed by the operator based on spec.tls.enabled.
	// +optional
	TLSDisable *bool `json:"tlsDisable,omitempty"`

	// ProxyProtocolBehavior allows configuring proxy protocol (e.g. for LoadBalancers).
	// +kubebuilder:validation:Enum=use_always;allow_any;deny_unauthorized
	// +optional
	ProxyProtocolBehavior string `json:"proxyProtocolBehavior,omitempty"`
}

// RaftConfig allows tuning the Raft storage backend.
type RaftConfig struct {
	// PerformanceMultiplier scales the Raft timing parameters.
	// +kubebuilder:validation:Minimum=0
	// +optional
	PerformanceMultiplier *int32 `json:"performanceMultiplier,omitempty"`

	// Autopilot configures Raft Autopilot settings.
	// By default, dead server cleanup is enabled with a 5-minute threshold.
	// +optional
	Autopilot *RaftAutopilotConfig `json:"autopilot,omitempty"`
}

// RaftAutopilotConfig configures Raft Autopilot behavior for dead server cleanup.
// See: https://openbao.org/docs/concepts/integrated-storage/autopilot/
type RaftAutopilotConfig struct {
	// CleanupDeadServers enables automatic removal of dead Raft peers.
	// When enabled, Autopilot periodically removes servers that have been
	// unhealthy for longer than DeadServerLastContactThreshold.
	// Requires MinQuorum to be set (defaults to replicas/2 + 1).
	// +kubebuilder:default=true
	// +optional
	CleanupDeadServers *bool `json:"cleanupDeadServers,omitempty"`

	// DeadServerLastContactThreshold is the duration after which a server
	// is considered dead if it hasn't contacted the leader.
	// Minimum: "1m". Default: "5m" (operator default, shorter than OpenBao's 24h).
	// +kubebuilder:default="5m"
	// +optional
	DeadServerLastContactThreshold string `json:"deadServerLastContactThreshold,omitempty"`

	// MinQuorum is the minimum number of servers before Autopilot can prune
	// dead servers. This prevents removing so many servers that quorum is lost.
	// If not specified, defaults to max(3, replicas/2 + 1).
	// +kubebuilder:validation:Minimum=3
	// +optional
	MinQuorum *int32 `json:"minQuorum,omitempty"`

	// ServerStabilizationTime is the minimum time a server must be healthy
	// before being promoted to voter. Default: "10s".
	// +optional
	ServerStabilizationTime string `json:"serverStabilizationTime,omitempty"`

	// LastContactThreshold is the limit on the amount of time a server can
	// go without leader contact before being considered unhealthy.
	// Default: "10s".
	// +optional
	LastContactThreshold string `json:"lastContactThreshold,omitempty"`

	// MaxTrailingLogs is the amount of entries in the Raft Log that a server
	// can be behind before being considered unhealthy. Default: 1000.
	// +optional
	MaxTrailingLogs *int32 `json:"maxTrailingLogs,omitempty"`
}

// LoggingConfig allows configuring logging behavior for OpenBao.
type LoggingConfig struct {
	// Format specifies the log format.
	// +kubebuilder:validation:Enum=standard;json
	// +optional
	Format string `json:"format,omitempty"`

	// File is the path to the log file.
	// If not specified, logs are written to stderr.
	// +optional
	File string `json:"file,omitempty"`

	// RotateDuration specifies how often to rotate logs (e.g., "24h", "7d").
	// +optional
	RotateDuration string `json:"rotateDuration,omitempty"`

	// RotateBytes specifies the maximum size in bytes before rotating logs.
	// +kubebuilder:validation:Minimum=0
	// +optional
	RotateBytes *int64 `json:"rotateBytes,omitempty"`

	// RotateMaxFiles is the maximum number of rotated log files to keep.
	// +kubebuilder:validation:Minimum=0
	// +optional
	RotateMaxFiles *int32 `json:"rotateMaxFiles,omitempty"`

	// PIDFile is the path to write the PID file.
	// +optional
	PIDFile string `json:"pidFile,omitempty"`
}

// PluginConfig allows configuring plugin behavior.
type PluginConfig struct {
	// FileUID is the UID to use for plugin files.
	// +optional
	FileUID *int64 `json:"fileUID,omitempty"`

	// FilePermissions are the file permissions for plugin files (e.g., "0755").
	// +optional
	FilePermissions string `json:"filePermissions,omitempty"`

	// AutoDownload controls automatic plugin downloads from OCI registries.
	// +optional
	AutoDownload *bool `json:"autoDownload,omitempty"`

	// AutoRegister controls automatic plugin registration.
	// +optional
	AutoRegister *bool `json:"autoRegister,omitempty"`

	// DownloadBehavior controls whether OpenBao startup fails or continues when
	// declarative OCI plugin downloads fail. Valid values are "fail" and
	// "continue"; OpenBao defaults to "fail" when unset.
	// +kubebuilder:validation:Enum=fail;continue
	// +optional
	DownloadBehavior string `json:"downloadBehavior,omitempty"`
}

// OpenBaoClusterSpec defines the desired state of an OpenBaoCluster.
// The Operator owns certain protected OpenBao configuration stanzas (for example,
// listener "tcp", storage "raft", and seal "static" when using default unseal).
// Users must not override these via spec.configuration.
// +kubebuilder:validation:XValidation:rule="self.tls.mode != 'OperatorManaged' || size(self.tls.rotationPeriod) > 0",message="spec.tls.rotationPeriod is required when spec.tls.mode is OperatorManaged"
// +kubebuilder:validation:XValidation:rule="self.tls.mode == 'ACME' || !has(self.tls.acme) || !has(self.tls.acme.sharedCache)",message="spec.tls.acme.sharedCache is only supported when spec.tls.mode is ACME"
// +kubebuilder:validation:XValidation:rule="self.tls.mode != 'ACME' || ((self.replicas <= 1) && (!has(self.upgrade) || self.upgrade.strategy != 'BlueGreen')) || (has(self.tls.acme) && has(self.tls.acme.sharedCache))",message="HA ACME clusters require spec.tls.acme.sharedCache when more than one Pod can serve the same hostname"
// +kubebuilder:validation:XValidation:rule="((!has(self.upgrade) || !has(self.upgrade.strategy) || size(self.upgrade.strategy) == 0) ? 'RollingUpdate' : self.upgrade.strategy) == ((!has(oldSelf.upgrade) || !has(oldSelf.upgrade.strategy) || size(oldSelf.upgrade.strategy) == 0) ? 'RollingUpdate' : oldSelf.upgrade.strategy)",message="spec.upgrade.strategy is immutable after creation; switching between RollingUpdate and BlueGreen is not supported."
// +kubebuilder:validation:XValidation:rule="!has(self.unseal) || self.unseal.type != 'ocikms' || !has(self.unseal.credentialsSecretRef) || (has(self.unseal.ocikms) && has(self.unseal.ocikms.authTypeAPIKey) && self.unseal.ocikms.authTypeAPIKey == true)",message="spec.unseal.credentialsSecretRef for ocikms requires spec.unseal.ocikms.authTypeAPIKey=true"
type OpenBaoClusterSpec struct {
	// Version is the semantic OpenBao version, used for upgrade orchestration.
	// The Operator uses static auto-unseal, which requires OpenBao v2.4.0 or later.
	// Versions below 2.4.0 do not support the static seal feature and will fail to start.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
	// Image is the container image to run; defaults may be derived from Version.
	// +optional
	Image string `json:"image,omitempty"`

	// ServiceAccount configures the Kubernetes ServiceAccount used by the OpenBao Pods.
	// +optional
	ServiceAccount *ServiceAccountConfig `json:"serviceAccount,omitempty"`
	// PodMetadata configures additional labels and annotations for the OpenBao Pod template.
	// This is useful for platform integrations that select Pods via metadata, such as
	// Azure Workload Identity. Operator-managed Pod metadata takes precedence.
	// +optional
	PodMetadata *PodMetadataConfig `json:"podMetadata,omitempty"`

	// ImagePullSecrets is a list of references to secrets in the same namespace
	// to use for pulling any images used by this Cluster (server, init, sidecars).
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Observability configures telemetry and metrics integration.
	// +optional
	Observability *ObservabilityConfig `json:"observability,omitempty"`
	// Replicas is the desired number of quorum-carrying voter Pods.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3
	Replicas int32 `json:"replicas"`
	// ReadReplicas configures the steady-state non-voter read-replica pool.
	// +optional
	ReadReplicas *ReadReplicaConfig `json:"readReplicas,omitempty"`
	// Paused, when true, pauses reconciliation for this OpenBaoCluster (except delete and finalizers).
	// +optional
	Paused bool `json:"paused,omitempty"`
	// Maintenance configures supported maintenance workflows.
	// +optional
	Maintenance *MaintenanceConfig `json:"maintenance,omitempty"`
	// Runtime configures explicit runtime control requests for the OpenBao workload.
	// +optional
	Runtime *RuntimeConfig `json:"runtime,omitempty"`
	// BreakGlassAck is an explicit acknowledgment token used to exit Break Glass / Safe Mode.
	//
	// When the operator enters break glass mode, it writes a nonce to status.breakGlass.nonce.
	// To acknowledge and allow the operator to resume quorum-risk automation, set this field
	// to match that nonce.
	//
	// Example:
	//   kubectl -n <ns> patch openbaocluster <name> --type merge -p '{"spec":{"breakGlassAck":"<nonce>"}}'
	//
	// +optional
	BreakGlassAck string `json:"breakGlassAck,omitempty"`
	// TLS configures TLS for the cluster.
	TLS TLSConfig `json:"tls"`
	// Storage configures persistent storage for the cluster.
	Storage StorageConfig `json:"storage"`
	// Service configures the primary Service used to expose OpenBao inside or outside the cluster.
	// +optional
	Service *ServiceConfig `json:"service,omitempty"`
	// Ingress configures optional HTTP(S) ingress in front of the OpenBao Service.
	// +optional
	Ingress *IngressConfig `json:"ingress,omitempty"`
	// Configuration defines the server configuration.
	// +optional
	Configuration *OpenBaoConfiguration `json:"configuration,omitempty"`
	// Backup configures scheduled backups for the cluster.
	// +optional
	Backup *BackupSchedule `json:"backup,omitempty"`
	// Restore configures optional restore authentication bootstrap for the cluster.
	// +optional
	Restore *RestoreConfig `json:"restore,omitempty"`
	// DeletionPolicy controls what happens to underlying resources when the CR is deleted.
	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
	// SelfInit configures OpenBao's native self-initialization feature.
	// When enabled, OpenBao initializes itself on first start using the configured
	// requests, and the root token is automatically revoked.
	// See: https://openbao.org/docs/configuration/self-init/
	// +optional
	SelfInit *SelfInitConfig `json:"selfInit,omitempty"`
	// Gateway configures Kubernetes Gateway API access (alternative to Ingress).
	// When enabled, the Operator creates an HTTPRoute that routes traffic through
	// a user-managed Gateway resource.
	// +optional
	Gateway *GatewayConfig `json:"gateway,omitempty"`
	// Network configures network-related settings for the cluster.
	// +optional
	Network *NetworkConfig `json:"network,omitempty"`
	// InitContainer configures the init container used to render OpenBao configuration.
	// The init container renders the final config.hcl from a template using environment
	// variables such as HOSTNAME and POD_IP.
	// +optional
	InitContainer *InitContainerConfig `json:"initContainer,omitempty"`
	// Audit configures declarative audit devices for the OpenBao cluster.
	// See: https://openbao.org/docs/configuration/audit/
	// +optional
	// +listType=map
	// +listMapKey=path
	Audit []AuditDevice `json:"audit,omitempty"`
	// AuditFileStorage configures a shared filesystem integration point for file audit devices.
	// When configured, file audit device paths must be under auditFileStorage.mountPath.
	// +optional
	AuditFileStorage *AuditFileStorageConfig `json:"auditFileStorage,omitempty"`
	// Plugins configures declarative plugins for the OpenBao cluster.
	// See: https://openbao.org/docs/configuration/plugins/
	// +optional
	Plugins []Plugin `json:"plugins,omitempty"`
	// Telemetry configures telemetry reporting for the OpenBao cluster.
	// See: https://openbao.org/docs/configuration/telemetry/
	// +optional
	Telemetry *TelemetryConfig `json:"telemetry,omitempty"`
	// Upgrade configures upgrade operations.
	//
	// Built-in upgrade executor Jobs authenticate with JWT auth using the
	// upgrade ServiceAccount (<cluster-name>-upgrade-serviceaccount). If
	// spec.selfInit.oidc.enabled is true and spec.upgrade.jwtAuthRole is empty,
	// the operator assumes or bootstraps the default "openbao-operator-upgrade"
	// role.
	//
	// Pre-upgrade snapshots use spec.backup configuration and backup
	// authentication rather than spec.upgrade credentials.
	// +optional
	Upgrade *UpgradeConfig `json:"upgrade,omitempty"`
	// Unseal defines the auto-unseal configuration.
	// If omitted, defaults to "static" mode managed by the operator.
	// +optional
	Unseal *UnsealConfig `json:"unseal,omitempty"`
	// ImageVerification configures supply chain security checks.
	// +optional
	ImageVerification *ImageVerificationConfig `json:"imageVerification,omitempty"`
	// OperatorImageVerification configures supply chain security checks for operator-managed helper images
	// (init container, backup/upgrade/restore executors). These images are typically signed
	// by the operator project (e.g., dc-tec/openbao-operator) rather than the OpenBao upstream project.
	// If omitted, helper image verification does not fall back to ImageVerification.
	// In Development, omitted means disabled. In Hardened, omitted means enabled.
	// +optional
	OperatorImageVerification *ImageVerificationConfig `json:"operatorImageVerification,omitempty"`
	// WorkloadHardening configures opt-in workload hardening features.
	// +optional
	WorkloadHardening *WorkloadHardeningConfig `json:"workloadHardening,omitempty"`
	// SecurityContext allows specifying the PodSecurityContext for the OpenBao Pods.
	// If set, these values override the default security context generated by the operator.
	// This is useful for OpenShift (SCC) compatibility or custom security requirements.
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	// Profile defines the security posture for this cluster.
	// When set to "Hardened", the operator enforces strict security requirements:
	// - TLS must be External (cert-manager/CSI managed)
	// - Unseal must use external KMS (no static unseal)
	// - SelfInit must be enabled (no root token)
	// When set to "Development", relaxed security is allowed but a security warning
	// condition is set.
	// +kubebuilder:validation:Enum=Hardened;Development
	Profile Profile `json:"profile"`
}

// UpgradeProgress tracks the state of an in-progress upgrade.
type UpgradeProgress struct {
	// TargetVersion is the version being upgraded to.
	TargetVersion string `json:"targetVersion"`
	// FromVersion is the version being upgraded from.
	FromVersion string `json:"fromVersion"`
	// StartedAt is when the upgrade began.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// CurrentPartition is the current StatefulSet partition value.
	CurrentPartition int32 `json:"currentPartition"`
	// CompletedPods lists ordinals of pods that have been successfully upgraded.
	// +optional
	CompletedPods []int32 `json:"completedPods,omitempty"`
	// LastStepDownTime records when the last leader step-down was performed.
	// +optional
	LastStepDownTime *metav1.Time `json:"lastStepDownTime,omitempty"`
	// Failure is the structured rolling-upgrade failure status.
	// When Failure.Reason is non-empty, the upgrade is considered failed.
	// +optional
	// +nullable
	// +kubebuilder:validation:Nullable
	Failure *ControllerErrorStatus `json:"failure,omitempty"`
	// LastErrorReason is a low-cardinality reason describing why the upgrade failed (if it did).
	// Deprecated: use Failure.Reason.
	// When set, the status controller should consider the cluster Degraded.
	// +optional
	LastErrorReason string `json:"lastErrorReason,omitempty"`
	// LastErrorMessage is a human-readable failure message (best-effort).
	// Deprecated: use Failure.Message.
	// +optional
	LastErrorMessage string `json:"lastErrorMessage,omitempty"`
	// LastErrorAt is when the last upgrade error was recorded (best-effort).
	// Deprecated: use Failure.At.
	// +optional
	LastErrorAt *metav1.Time `json:"lastErrorAt,omitempty"`
}

// ControllerErrorStatus captures a controller-scoped error signal that the status controller
// can translate into high-level conditions.
type ControllerErrorStatus struct {
	// Reason is a low-cardinality identifier for the error.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable error message (best-effort).
	// +optional
	Message string `json:"message,omitempty"`
	// At is when the error was observed (best-effort).
	// +optional
	At *metav1.Time `json:"at,omitempty"`
}

// WorkloadControllerStatus holds status owned by the workload controller.
type WorkloadControllerStatus struct {
	// LastError is the last workload-controller error observed for this cluster.
	// +optional
	// +nullable
	// +kubebuilder:validation:Nullable
	LastError *ControllerErrorStatus `json:"lastError,omitempty"`
}

// AdminOpsControllerStatus holds status owned by the adminops controller.
type AdminOpsControllerStatus struct {
	// LastError is the last adminops-controller error observed for this cluster.
	// +optional
	// +nullable
	// +kubebuilder:validation:Nullable
	LastError *ControllerErrorStatus `json:"lastError,omitempty"`
}

// BlueGreenPhase is a high-level summary of blue/green upgrade state.
// +kubebuilder:validation:Enum=Idle;DeployingGreen;JoiningMesh;Syncing;Promoting;DemotingBlue;Cleanup;RestoringReadReplicas;RollingBack;RollbackCleanup
type BlueGreenPhase string

const (
	// PhaseIdle indicates no blue/green upgrade is in progress.
	PhaseIdle BlueGreenPhase = "Idle"
	// PhaseDeployingGreen indicates the Green StatefulSet is being created and pods are becoming ready.
	// This phase includes waiting for pods to be unsealed.
	PhaseDeployingGreen BlueGreenPhase = "DeployingGreen"
	// PhaseJoiningMesh indicates Green pods are joining the Raft cluster as non-voters.
	PhaseJoiningMesh BlueGreenPhase = "JoiningMesh"
	// PhaseSyncing indicates waiting for Green nodes to catch up with Blue nodes.
	PhaseSyncing BlueGreenPhase = "Syncing"
	// PhasePromoting indicates Green nodes are being promoted to voters.
	PhasePromoting BlueGreenPhase = "Promoting"
	// PhaseDemotingBlue indicates Blue nodes are being demoted to non-voters.
	PhaseDemotingBlue BlueGreenPhase = "DemotingBlue"
	// PhaseCleanup indicates Blue StatefulSet is being deleted.
	PhaseCleanup BlueGreenPhase = "Cleanup"
	// PhaseRestoringReadReplicas indicates the steady-state read-replica pool is
	// being restored after cutover cleanup and must converge before the upgrade
	// returns to Idle.
	PhaseRestoringReadReplicas BlueGreenPhase = "RestoringReadReplicas"
	// PhaseRollingBack indicates the upgrade is being rolled back.
	// Blue nodes are re-promoted and Green nodes are demoted.
	PhaseRollingBack BlueGreenPhase = "RollingBack"
	// PhaseRollbackCleanup indicates Green StatefulSet is being deleted after rollback.
	PhaseRollbackCleanup BlueGreenPhase = "RollbackCleanup"
)

// BlueGreenStatus tracks the lifecycle of the "Green" revision during blue/green upgrades.
type BlueGreenStatus struct {
	// Phase is the current phase of the blue/green upgrade.
	Phase BlueGreenPhase `json:"phase,omitempty"`
	// BlueRevision is the hash/name of the currently active cluster.
	BlueRevision string `json:"blueRevision,omitempty"`
	// BlueImage is the container image used by the Blue cluster.
	// This ensures the Blue cluster is not actively upgraded when spec.image changes.
	BlueImage string `json:"blueImage,omitempty"`
	// GreenRevision is the hash/name of the next cluster (if upgrade in progress).
	GreenRevision string `json:"greenRevision,omitempty"`
	// ManualPromotionRequired snapshots whether the current in-flight blue/green
	// upgrade requires an explicit spec.upgrade.requests.promote request before
	// promotion can proceed. It is derived from spec.upgrade.blueGreen.autoPromote
	// when the upgrade starts.
	// +optional
	ManualPromotionRequired bool `json:"manualPromotionRequired,omitempty"`
	// StartTime is when the current phase began.
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// JobFailureCount tracks consecutive job failures in the current phase.
	// Reset to 0 on phase transition or successful job completion.
	// +optional
	JobFailureCount int32 `json:"jobFailureCount,omitempty"`
	// LastJobFailure records the name of the last failed job for debugging.
	// +optional
	LastJobFailure string `json:"lastJobFailure,omitempty"`
	// PreUpgradeSnapshotJobName is the name of the backup job triggered at upgrade start.
	// +optional
	PreUpgradeSnapshotJobName string `json:"preUpgradeSnapshotJobName,omitempty"`
	// RollbackReason records why a rollback was triggered (if any).
	// +optional
	RollbackReason string `json:"rollbackReason,omitempty"`
	// RollbackStartTime is when the rollback was initiated.
	// +optional
	RollbackStartTime *metav1.Time `json:"rollbackStartTime,omitempty"`
	// RollbackAttempt increments each time rollback automation is retried.
	// It is used to produce stable, deterministic Job names per attempt.
	// +optional
	RollbackAttempt int32 `json:"rollbackAttempt,omitempty"`
}

// UpgradeRequestStatus tracks which explicit upgrade request values have already been handled.
type UpgradeRequestStatus struct {
	// LastHandledRetry is the last observed spec.upgrade.requests.retry value
	// that the operator has handled.
	// +optional
	LastHandledRetry string `json:"lastHandledRetry,omitempty"`
	// LastHandledPromote is the last observed spec.upgrade.requests.promote
	// value that the operator has handled.
	// +optional
	LastHandledPromote string `json:"lastHandledPromote,omitempty"`
	// LastHandledRollback is the last observed spec.upgrade.requests.rollback
	// value that the operator has handled.
	// +optional
	LastHandledRollback string `json:"lastHandledRollback,omitempty"`
}

// BackupStatus tracks the state of backups for a cluster.
type BackupStatus struct {
	// LastBackupTime is the timestamp of the last successful backup.
	// +optional
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
	// LastAttemptTime is the timestamp of the last backup attempt, regardless of outcome.
	// This is used to avoid retry loops when a scheduled backup fails.
	// +optional
	LastAttemptTime *metav1.Time `json:"lastAttemptTime,omitempty"`
	// LastAttemptScheduledTime is the scheduled time of the last backup attempt.
	// It is derived from the cron schedule and used to ensure at-most-once execution
	// per scheduled window.
	// +optional
	LastAttemptScheduledTime *metav1.Time `json:"lastAttemptScheduledTime,omitempty"`
	// LastHandledManualTrigger is the last observed manual trigger token that
	// has progressed into an actual backup attempt.
	// +optional
	LastHandledManualTrigger string `json:"lastHandledManualTrigger,omitempty"`
	// LastBackupSize is the size in bytes of the last successful backup.
	// +optional
	LastBackupSize int64 `json:"lastBackupSize,omitempty"`
	// LastBackupDuration is how long the last backup took (e.g., "45s").
	// +optional
	LastBackupDuration string `json:"lastBackupDuration,omitempty"`
	// LastBackupName is the object key/path of the last successful backup.
	// +optional
	LastBackupName string `json:"lastBackupName,omitempty"`
	// NextScheduledBackup is when the next backup is scheduled.
	// +optional
	NextScheduledBackup *metav1.Time `json:"nextScheduledBackup,omitempty"`
	// ConsecutiveFailures is the number of consecutive backup failures.
	// +optional
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`
	// LastFailureReason is the low-cardinality reason code for the last backup failure (if applicable).
	// +optional
	LastFailureReason string `json:"lastFailureReason,omitempty"`
	// LastFailureMessage is the detailed message for the last backup failure (if applicable).
	// +optional
	LastFailureMessage string `json:"lastFailureMessage,omitempty"`
	// LastFailureTime is when the last backup failure was recorded.
	// +optional
	LastFailureTime *metav1.Time `json:"lastFailureTime,omitempty"`
}

// ReadReplicaStorageStatus captures observed storage state for the read-replica
// pool.
type ReadReplicaStorageStatus struct {
	// DesiredPVCs is the number of data PVCs expected for the read-replica pool.
	// +optional
	DesiredPVCs int32 `json:"desiredPVCs,omitempty"`
	// BoundPVCs is the number of observed data PVCs for the read-replica pool.
	// +optional
	BoundPVCs int32 `json:"boundPVCs,omitempty"`
	// StorageClassName is the effective StorageClass observed for the
	// read-replica PVCs when it is consistent.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`
}

// ReadReplicaStatus captures observed state for the read-replica pool.
type ReadReplicaStatus struct {
	// DesiredReplicas is the desired number of read replicas.
	// +optional
	DesiredReplicas int32 `json:"desiredReplicas,omitempty"`
	// ReadyReplicas is the number of Ready read-replica Pods observed.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// RegisteredReplicas is the number of observed non-voter peers registered in
	// Raft membership.
	// +optional
	RegisteredReplicas int32 `json:"registeredReplicas,omitempty"`
	// HealthyReplicas is the number of read-replica peers that are currently
	// healthy according to the Raft Autopilot state endpoint.
	// +optional
	HealthyReplicas int32 `json:"healthyReplicas,omitempty"`
	// Storage captures read-replica-specific storage observation state.
	// +optional
	Storage ReadReplicaStorageStatus `json:"storage,omitempty"`
}

// DriftStatus tracks drift detection and correction events for a cluster.
// OpenBaoClusterStatus defines the observed state of an OpenBaoCluster.
type OpenBaoClusterStatus struct {
	// ObservedGeneration is the most recent metadata.generation that has been
	// reconciled into this status.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Phase is a high-level summary of the cluster state.
	// +optional
	Phase ClusterPhase `json:"phase,omitempty"`
	// ActiveLeader is the current Raft leader pod name, for example "prod-cluster-0".
	// +optional
	ActiveLeader string `json:"activeLeader,omitempty"`
	// ReadyReplicas is the number of replicas that are currently Ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// ReadReplicas captures observed state for the read-replica pool.
	// +optional
	// +nullable
	// +kubebuilder:validation:Nullable
	ReadReplicas *ReadReplicaStatus `json:"readReplicas,omitempty"`
	// CurrentVersion is the OpenBao version currently running on the cluster.
	// +optional
	CurrentVersion string `json:"currentVersion,omitempty"`
	// Initialized indicates whether the OpenBao cluster has been initialized.
	// This is set to true after the first pod is initialized using bao operator init
	// or after self-initialization completes.
	// +optional
	Initialized bool `json:"initialized,omitempty"`
	// SelfInitialized indicates whether the cluster was initialized using
	// OpenBao's self-initialization feature. When true, no root token Secret
	// exists for this cluster (the root token was auto-revoked).
	// +optional
	SelfInitialized bool `json:"selfInitialized,omitempty"`
	// LastBackupTime is the timestamp of the last successful backup, if configured.
	// Deprecated: Use Backup.LastBackupTime instead.
	// +optional
	// +nullable
	// +kubebuilder:validation:Nullable
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
	// Upgrade tracks the state of an in-progress upgrade (if any).
	// When non-nil, an upgrade is in progress and the UpgradeManager is orchestrating
	// the pod-by-pod rolling update with leader step-down.
	// +optional
	// +kubebuilder:validation:Nullable
	Upgrade *UpgradeProgress `json:"upgrade,omitempty"`
	// UpgradeRequests tracks which explicit upgrade request values have already
	// been handled so one-shot requests are edge-triggered instead of level-triggered.
	// +optional
	// +kubebuilder:validation:Nullable
	UpgradeRequests *UpgradeRequestStatus `json:"upgradeRequests,omitempty"`
	// Backup tracks the state of backups for this cluster.
	// +optional
	// +kubebuilder:validation:Nullable
	Backup *BackupStatus `json:"backup,omitempty"`
	// BlueGreen tracks the state of blue/green upgrades (if enabled).
	// +optional
	// +kubebuilder:validation:Nullable
	BlueGreen *BlueGreenStatus `json:"blueGreen,omitempty"`
	// OperationLock prevents concurrent long-running operations (upgrade/backup/restore)
	// from acting on the same cluster at the same time.
	// +optional
	// +nullable
	// +kubebuilder:validation:Nullable
	OperationLock *OperationLockStatus `json:"operationLock,omitempty"`
	// BreakGlass records when the operator has halted quorum-risk automation and requires
	// explicit operator acknowledgment to continue.
	// +optional
	// +kubebuilder:validation:Nullable
	BreakGlass *BreakGlassStatus `json:"breakGlass,omitempty"`
	// Workload holds signals owned by the workload controller (infrastructure reconciliation).
	// +optional
	// +nullable
	// +kubebuilder:validation:Nullable
	Workload *WorkloadControllerStatus `json:"workload,omitempty"`
	// AdminOps holds signals owned by the adminops controller (upgrade + backup).
	// +optional
	// +nullable
	// +kubebuilder:validation:Nullable
	AdminOps *AdminOpsControllerStatus `json:"adminOps,omitempty"`
	// Conditions represent the current state of the OpenBaoCluster resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ClusterOperation identifies a mutually-exclusive operator operation.
// +kubebuilder:validation:Enum=Upgrade;Backup;Restore
type ClusterOperation string

const (
	ClusterOperationUpgrade ClusterOperation = "Upgrade"
	ClusterOperationBackup  ClusterOperation = "Backup"
	ClusterOperationRestore ClusterOperation = "Restore"
)

// OperationLockStatus represents a status-based lock held by the operator.
// +structType=atomic
type OperationLockStatus struct {
	// Operation is the operation currently holding the lock.
	// +optional
	Operation ClusterOperation `json:"operation,omitempty"`
	// Holder is a stable identifier for the lock holder (controller/component).
	// +optional
	Holder string `json:"holder,omitempty"`
	// Message provides human-readable context for why the lock is held.
	// +optional
	Message string `json:"message,omitempty"`
	// AcquiredAt is when the lock was first acquired.
	// +optional
	AcquiredAt *metav1.Time `json:"acquiredAt,omitempty"`
	// RenewedAt is updated when the holder reasserts the lock during reconciliation.
	// +optional
	RenewedAt *metav1.Time `json:"renewedAt,omitempty"`
}

// BreakGlassReason describes why the operator required manual intervention.
// +kubebuilder:validation:Enum=RollbackConsensusRepairFailed;RollbackCleanupPeerRemovalFailed
type BreakGlassReason string

const (
	BreakGlassReasonRollbackConsensusRepairFailed    BreakGlassReason = "RollbackConsensusRepairFailed"
	BreakGlassReasonRollbackCleanupPeerRemovalFailed BreakGlassReason = "RollbackCleanupPeerRemovalFailed"
)

// BreakGlassStatus captures safe-mode / break-glass state and recovery guidance.
type BreakGlassStatus struct {
	// Active indicates whether break glass mode is currently active.
	// +optional
	Active bool `json:"active,omitempty"`
	// Reason is a stable, typed reason for entering break glass mode.
	// +optional
	Reason BreakGlassReason `json:"reason,omitempty"`
	// Message provides a short summary of the detected unsafe state.
	// +optional
	Message string `json:"message,omitempty"`
	// Nonce is the acknowledgment token required to resume automation.
	// +optional
	Nonce string `json:"nonce,omitempty"`
	// EnteredAt is when break glass mode became active.
	// +optional
	EnteredAt *metav1.Time `json:"enteredAt,omitempty"`
	// Steps provides deterministic recovery guidance.
	// +optional
	Steps []string `json:"steps,omitempty"`
	// AcknowledgedAt records when break glass was acknowledged.
	// +optional
	AcknowledgedAt *metav1.Time `json:"acknowledgedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=openbaoclusters,scope=Namespaced,shortName=bao;baoc
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoCluster is the Schema for the openbaoclusters API.
type OpenBaoCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of OpenBaoCluster.
	Spec OpenBaoClusterSpec `json:"spec"`

	// Status defines the observed state of OpenBaoCluster.
	// +optional
	Status OpenBaoClusterStatus `json:"status"`
}

// +kubebuilder:object:root=true

// OpenBaoClusterList contains a list of OpenBaoCluster.
type OpenBaoClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []OpenBaoCluster `json:"items"`
}

// AuditDevice defines a declarative audit device configuration.
// See: https://openbao.org/docs/configuration/audit/
// +kubebuilder:validation:XValidation:rule="self.type == 'file' || !has(self.fileOptions)",message="fileOptions is only supported when type is file"
// +kubebuilder:validation:XValidation:rule="self.type == 'http' || !has(self.httpOptions)",message="httpOptions is only supported when type is http"
// +kubebuilder:validation:XValidation:rule="self.type == 'syslog' || !has(self.syslogOptions)",message="syslogOptions is only supported when type is syslog"
// +kubebuilder:validation:XValidation:rule="self.type == 'socket' || !has(self.socketOptions)",message="socketOptions is only supported when type is socket"
type AuditDevice struct {
	// Type is the type of audit device (e.g., "file", "syslog", "socket", "http").
	// +kubebuilder:validation:Enum=file;syslog;socket;http
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
	// Path is the path of the audit device in the root namespace.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// Description is an optional description for the audit device.
	// +optional
	Description string `json:"description,omitempty"`
	// FileOptions configures options for file audit devices.
	// Only used when Type is "file".
	// +optional
	FileOptions *FileAuditOptions `json:"fileOptions,omitempty"`
	// HTTPOptions configures options for HTTP audit devices.
	// Only used when Type is "http".
	// +optional
	HTTPOptions *HTTPAuditOptions `json:"httpOptions,omitempty"`
	// SyslogOptions configures options for syslog audit devices.
	// Only used when Type is "syslog".
	// +optional
	SyslogOptions *SyslogAuditOptions `json:"syslogOptions,omitempty"`
	// SocketOptions configures options for socket audit devices.
	// Only used when Type is "socket".
	// +optional
	SocketOptions *SocketAuditOptions `json:"socketOptions,omitempty"`
	// Options contains device-specific configuration options as a string map.
	// This is a fallback for backward compatibility and advanced use cases.
	// If structured options (FileOptions, HTTPOptions, etc.) are provided, they take precedence.
	// OpenBao audit options are string-to-string; scalar JSON values are rendered as strings,
	// while nested objects and arrays are rejected. For HTTP headers, prefer httpOptions.headers.
	// +optional
	Options *apiextensionsv1.JSON `json:"options,omitempty"`
}

// FileAuditOptions configures options for file audit devices.
// See: https://openbao.org/docs/audit/file/
type FileAuditOptions struct {
	// FilePath is the path to where the audit log will be written.
	// Special keywords: "stdout" writes to standard output, "discard" discards output.
	// +kubebuilder:validation:MinLength=1
	FilePath string `json:"filePath"`
	// Mode is a string containing an octal number representing the bit pattern for the file mode.
	// Defaults to "0600" if not specified. Set to "0000" to prevent OpenBao from modifying the file mode.
	// +optional
	Mode string `json:"mode,omitempty"`
}

// HTTPAuditOptions configures options for HTTP audit devices.
// See: https://openbao.org/docs/audit/http/
type HTTPAuditOptions struct {
	// URI is the URI of the remote server where the audit logs will be written.
	// +kubebuilder:validation:MinLength=1
	URI string `json:"uri"`
	// Headers is a JSON object describing headers. Must take the shape map[string][]string,
	// i.e., an object of headers, with each having one or more values.
	// Headers without values will be ignored. The operator renders this object as OpenBao's
	// expected JSON-encoded options.headers string.
	// +optional
	Headers *apiextensionsv1.JSON `json:"headers,omitempty"`
}

// SyslogAuditOptions configures options for syslog audit devices.
// See: https://openbao.org/docs/audit/syslog/
type SyslogAuditOptions struct {
	// Facility is the syslog facility to use.
	// Defaults to "AUTH" if not specified.
	// +optional
	Facility string `json:"facility,omitempty"`
	// Tag is the syslog tag to use.
	// Defaults to "openbao" if not specified.
	// +optional
	Tag string `json:"tag,omitempty"`
}

// SocketAuditOptions configures options for socket audit devices.
// See: https://openbao.org/docs/audit/socket/
type SocketAuditOptions struct {
	// Address is the socket server address to use.
	// Example: "127.0.0.1:9090" or "/tmp/audit.sock".
	// +optional
	Address string `json:"address,omitempty"`
	// SocketType is the socket type to use, any type compatible with net.Dial is acceptable.
	// Defaults to "tcp" if not specified.
	// +optional
	SocketType string `json:"socketType,omitempty"`
	// WriteTimeout is the (deadline) time in seconds to allow writes to be completed over the socket.
	// A zero value means that write attempts will not time out.
	// Defaults to "2s" if not specified.
	// +optional
	WriteTimeout string `json:"writeTimeout,omitempty"`
}

// Plugin defines a declarative plugin configuration.
// See: https://openbao.org/docs/configuration/plugins/
type Plugin struct {
	// Type is the plugin type (e.g., "secret", "auth").
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
	// Name is the name of the plugin.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Image is the OCI image URL including registry and repository.
	// Required if Command is not set. Conflicts with Command.
	// +optional
	Image string `json:"image,omitempty"`
	// Command is the command name of a manually downloaded plugin.
	// Required if Image is not set. Conflicts with Image.
	// +optional
	Command string `json:"command,omitempty"`
	// Version is the image version or tag.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
	// BinaryName is the name of the plugin binary file within the OCI image.
	// +kubebuilder:validation:MinLength=1
	BinaryName string `json:"binaryName"`
	// SHA256Sum is the expected SHA256 checksum of the plugin binary.
	// Must be a 64-character hexadecimal string.
	// +kubebuilder:validation:MinLength=64
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern=`^[0-9a-fA-F]{64}$`
	SHA256Sum string `json:"sha256sum"`
	// Args are arguments to pass to the running plugin.
	// Only used if plugin_auto_register=true is set.
	// +optional
	Args []string `json:"args,omitempty"`
	// Env are environment variables to pass to the running plugin.
	// Only used if plugin_auto_register=true is set.
	// +optional
	Env []string `json:"env,omitempty"`
}

// TelemetryConfig defines telemetry reporting configuration.
// See: https://openbao.org/docs/configuration/telemetry/
type TelemetryConfig struct {
	// Common telemetry options
	// UsageGaugePeriod specifies the interval at which high-cardinality usage data is collected.
	// +optional
	UsageGaugePeriod string `json:"usageGaugePeriod,omitempty"`
	// MaximumGaugeCardinality is the maximum cardinality of gauge labels.
	// +optional
	MaximumGaugeCardinality *int32 `json:"maximumGaugeCardinality,omitempty"`
	// DisableHostname specifies if gauge values should be prefixed with the local hostname.
	// +optional
	DisableHostname bool `json:"disableHostname,omitempty"`
	// EnableHostnameLabel specifies if all metric values should contain the host label.
	// +optional
	EnableHostnameLabel bool `json:"enableHostnameLabel,omitempty"`
	// MetricsPrefix specifies the prefix used for metric values.
	// +optional
	MetricsPrefix string `json:"metricsPrefix,omitempty"`
	// LeaseMetricsEpsilon specifies the size of the bucket used to measure future lease expiration.
	// +optional
	LeaseMetricsEpsilon string `json:"leaseMetricsEpsilon,omitempty"`

	// Prometheus-specific options
	// PrometheusRetentionTime specifies how long to retain metrics in Prometheus format.
	// +optional
	PrometheusRetentionTime string `json:"prometheusRetentionTime,omitempty"`

	// Statsite-specific options
	// StatsiteAddress is the address of the statsite server.
	// +optional
	StatsiteAddress string `json:"statsiteAddress,omitempty"`

	// StatsD-specific options
	// StatsdAddress is the address of the StatsD server.
	// +optional
	StatsdAddress string `json:"statsdAddress,omitempty"`

	// DogStatsD-specific options
	// DogStatsdAddress is the address of the DogStatsD server.
	// +optional
	DogStatsdAddress string `json:"dogStatsdAddress,omitempty"`
	// DogStatsdTags are tags to add to all metrics.
	// +optional
	DogStatsdTags []string `json:"dogStatsdTags,omitempty"`

	// Circonus-specific options
	// CirconusAPIKey is the API key for Circonus.
	// +optional
	CirconusAPIKey string `json:"circonusAPIKey,omitempty"`
	// CirconusAPIApp is the API app name for Circonus.
	// +optional
	CirconusAPIApp string `json:"circonusAPIApp,omitempty"`
	// CirconusAPIURL is the API URL for Circonus.
	// +optional
	CirconusAPIURL string `json:"circonusAPIURL,omitempty"`
	// CirconusSubmissionInterval is the submission interval for Circonus.
	// +optional
	CirconusSubmissionInterval string `json:"circonusSubmissionInterval,omitempty"`
	// CirconusCheckID is the check ID for Circonus.
	// +optional
	CirconusCheckID string `json:"circonusCheckID,omitempty"`
	// CirconusCheckForceMetricActivation forces metric activation in Circonus.
	// +optional
	CirconusCheckForceMetricActivation string `json:"circonusCheckForceMetricActivation,omitempty"`
	// CirconusCheckInstanceID is the instance ID for Circonus.
	// +optional
	CirconusCheckInstanceID string `json:"circonusCheckInstanceID,omitempty"`
	// CirconusCheckSearchTag is the search tag for Circonus.
	// +optional
	CirconusCheckSearchTag string `json:"circonusCheckSearchTag,omitempty"`
	// CirconusCheckDisplayName is the display name for Circonus.
	// +optional
	CirconusCheckDisplayName string `json:"circonusCheckDisplayName,omitempty"`
	// CirconusCheckTags is the tags for Circonus.
	// +optional
	CirconusCheckTags string `json:"circonusCheckTags,omitempty"`
	// CirconusBrokerID is the broker ID for Circonus.
	// +optional
	CirconusBrokerID string `json:"circonusBrokerID,omitempty"`
	// CirconusBrokerSelectTag is the broker select tag for Circonus.
	// +optional
	CirconusBrokerSelectTag string `json:"circonusBrokerSelectTag,omitempty"`

	// Stackdriver-specific options
	// StackdriverProjectID is the Google Cloud Project ID.
	// +optional
	StackdriverProjectID string `json:"stackdriverProjectID,omitempty"`
	// StackdriverLocation is the GCP or AWS region.
	// +optional
	StackdriverLocation string `json:"stackdriverLocation,omitempty"`
	// StackdriverNamespace is a namespace identifier for the telemetry data.
	// +optional
	StackdriverNamespace string `json:"stackdriverNamespace,omitempty"`
	// StackdriverDebugLogs specifies if OpenBao writes additional stackdriver debug logs.
	// +optional
	StackdriverDebugLogs bool `json:"stackdriverDebugLogs,omitempty"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoCluster{}, &OpenBaoClusterList{})
}

// ServiceAccountConfig configures the ServiceAccount used by OpenBao pods.
type ServiceAccountConfig struct {
	// Name overrides the generated ServiceAccount name.
	// If not specified, defaults to "<cluster-name>-serviceaccount".
	// +optional
	Name string `json:"name,omitempty"`

	// Annotations to add to the ServiceAccount.
	// Useful for cloud provider Workload Identity (e.g. eks.amazonaws.com/role-arn).
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PodMetadataConfig configures additional metadata for the OpenBao Pod template.
type PodMetadataConfig struct {
	// Labels are merged into the generated OpenBao Pod template labels.
	// Operator-managed labels take precedence if the same key is specified here.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are merged into the generated OpenBao Pod template annotations.
	// Operator-managed annotations take precedence if the same key is specified here.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ObservabilityConfig configures observability features.
type ObservabilityConfig struct {
	// Metrics configures integration with Prometheus/OpenMetrics.
	// +optional
	Metrics *MetricsConfig `json:"metrics,omitempty"`
}

// MetricsConfig configures metrics collection.
type MetricsConfig struct {
	// Enabled configures the OpenBao telemetry stanza and creates a ServiceMonitor.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// ScrapeProfile selects which OpenBao pods are targeted by generated scrape resources.
	// Active targets only the active OpenBao pod. AllNodes targets every OpenBao pod and
	// requires a dedicated metrics-only listener.
	// +kubebuilder:validation:Enum=Active;AllNodes
	// +kubebuilder:default=Active
	// +optional
	ScrapeProfile string `json:"scrapeProfile,omitempty"`

	// MetricsOnlyListener configures a dedicated listener for metrics scraping.
	// It is enabled automatically when scrapeProfile is AllNodes.
	// +optional
	MetricsOnlyListener *MetricsOnlyListenerConfig `json:"metricsOnlyListener,omitempty"`

	// ServiceMonitor controls whether to create a Prometheus Operator ServiceMonitor.
	// +optional
	ServiceMonitor *ServiceMonitorConfig `json:"serviceMonitor,omitempty"`
}

// MetricsOnlyListenerConfig configures a dedicated metrics-only TCP listener.
type MetricsOnlyListenerConfig struct {
	// Enabled controls whether to render the dedicated metrics-only listener.
	// When omitted, the listener is enabled automatically for the AllNodes scrape profile.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Port is the dedicated metrics listener port.
	// +kubebuilder:default=8202
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// UnauthenticatedMetricsAccess allows unauthenticated access to /v1/sys/metrics
	// on the metrics-only listener. AllNodes scraping needs this so standby nodes can
	// expose metrics. Restrict this listener with NetworkPolicy.
	// +optional
	UnauthenticatedMetricsAccess *bool `json:"unauthenticatedMetricsAccess,omitempty"`
}

// ServiceMonitorConfig configures the Prometheus ServiceMonitor.
type ServiceMonitorConfig struct {
	// Enabled controls whether to create the ServiceMonitor.
	// Defaults to true if Metrics are enabled.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// Interval is the scrape interval.
	// +kubebuilder:default="30s"
	// +optional
	Interval string `json:"interval,omitempty"`

	// ScrapeTimeout is the scrape timeout.
	// +kubebuilder:default="10s"
	// +optional
	ScrapeTimeout string `json:"scrapeTimeout,omitempty"`

	// Labels are added to the ServiceMonitor metadata.
	// Use this for Prometheus selectors, such as release labels used by kube-prometheus-stack.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are added to the ServiceMonitor metadata.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// JobLabel selects the Service label Prometheus uses as the job label.
	// Defaults to app.kubernetes.io/name.
	// +optional
	JobLabel string `json:"jobLabel,omitempty"`

	// Authorization configures an optional ServiceMonitor authorization block.
	// Use this for authenticated /v1/sys/metrics scraping.
	// +optional
	Authorization *ServiceMonitorAuthorizationConfig `json:"authorization,omitempty"`

	// TLSConfig configures TLS verification for the OpenBao scrape endpoint.
	// +optional
	TLSConfig *ServiceMonitorTLSConfig `json:"tlsConfig,omitempty"`
}

// ServiceMonitorAuthorizationConfig configures Prometheus Operator endpoint authorization.
type ServiceMonitorAuthorizationConfig struct {
	// Type is the authorization type.
	// Defaults to Bearer when credentialsSecret is set.
	// +optional
	Type string `json:"type,omitempty"`

	// CredentialsSecret references a Secret key containing the authorization credentials.
	// The Secret must exist in the same namespace as the ServiceMonitor.
	CredentialsSecret ServiceMonitorKeySelector `json:"credentialsSecret"`
}

// ServiceMonitorTLSConfig configures TLS settings for the Prometheus Operator endpoint.
type ServiceMonitorTLSConfig struct {
	// ServerName verifies the hostname in the OpenBao serving certificate.
	// +optional
	ServerName string `json:"serverName,omitempty"`

	// InsecureSkipVerify disables TLS certificate verification.
	// Use only for temporary non-production environments.
	// +optional
	InsecureSkipVerify *bool `json:"insecureSkipVerify,omitempty"`

	// CAConfigMap references a ConfigMap key containing the CA certificate.
	// Mutually exclusive with CASecret.
	// +optional
	CAConfigMap *ServiceMonitorKeySelector `json:"caConfigMap,omitempty"`

	// CASecret references a Secret key containing the CA certificate.
	// Mutually exclusive with CAConfigMap.
	// +optional
	CASecret *ServiceMonitorKeySelector `json:"caSecret,omitempty"`
}

// ServiceMonitorKeySelector identifies a key in a Secret or ConfigMap.
type ServiceMonitorKeySelector struct {
	// Name is the Secret or ConfigMap name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key is the key within the Secret or ConfigMap.
	// Defaults to token for authorization credentials and ca.crt for CA references.
	// +optional
	Key string `json:"key,omitempty"`
}

// SelfInitOIDCConfig configures OIDC identity for the cluster.
type SelfInitOIDCConfig struct {
	// Enabled triggers the bootstrap logic.
	Enabled bool `json:"enabled"`

	// Audience, if set, must match the operator installation audience used for
	// projected OpenBao auth tokens.
	// This field does not create a per-cluster TokenRequest audience override.
	// +optional
	Audience string `json:"audience,omitempty"`

	// Issuer overrides the auto-discovered K8s issuer URL.
	// Critical for scenarios where OpenBao sees a different K8s URL than the Operator.
	// +optional
	Issuer string `json:"issuer,omitempty"`
}
