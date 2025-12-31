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
	// ConditionProductionReady indicates whether the cluster configuration is considered
	// production-ready by the operator (security posture, unseal, bootstrap flow).
	ConditionProductionReady ConditionType = "ProductionReady"
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
type ACMEConfig struct {
	// DirectoryURL is the ACME directory URL (e.g., "https://acme-v02.api.letsencrypt.org/directory").
	// +kubebuilder:validation:MinLength=1
	DirectoryURL string `json:"directoryURL"`
	// Domain is the domain name for which to obtain the certificate.
	// +kubebuilder:validation:MinLength=1
	Domain string `json:"domain"`
	// Email is the email address to use for ACME registration.
	// +optional
	Email string `json:"email,omitempty"`
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
	// TLSSecretName is an optional TLS Secret name; when empty the cluster TLS Secret is used.
	// +optional
	TLSSecretName string `json:"tlsSecretName,omitempty"`
	// Annotations are additional annotations to apply to the Ingress.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
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
	// +optional
	JWTAuthRole string `json:"jwtAuthRole,omitempty"`
	// TokenSecretRef optionally references a Secret containing an OpenBao API
	// token to use for backup operations (fallback method).
	//
	// For standard clusters (non self-init), this is typically omitted and the
	// operator uses the root token from <cluster>-root-token. For self-init
	// clusters (no root token Secret), this field must reference a token with
	// permission to read sys/storage/raft/snapshot.
	//
	// If JWTAuthRole is set, this field is ignored in favor of JWT Auth.
	// +optional
	TokenSecretRef *corev1.SecretReference `json:"tokenSecretRef,omitempty"`
	// Retention defines optional backup retention policy.
	// +optional
	Retention *BackupRetention `json:"retention,omitempty"`
	// ExecutorImage is the container image to use for backup operations.
	// Defaults to "openbao/backup-executor:v0.1.0" if not specified.
	// This allows users to override the image for air-gapped environments or custom registries.
	// +kubebuilder:validation:MinLength=1
	// +optional
	ExecutorImage string `json:"executorImage,omitempty"`
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
	// If the job fails, the upgrade enters a paused state until manually resolved.
	// +optional
	PrePromotionHook *ValidationHookConfig `json:"prePromotionHook,omitempty"`

	// PostSwitchHook specifies a Job template to run after switching traffic
	// to the Green cluster. The job must complete successfully (exit 0) before
	// the upgrade proceeds beyond the TrafficSwitching phase. If the job fails
	// and auto-rollback on traffic failure is enabled, the operator triggers
	// a rollback; otherwise, the upgrade remains paused in TrafficSwitching
	// until manually resolved.
	// +optional
	PostSwitchHook *ValidationHookConfig `json:"postSwitchHook,omitempty"`
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
	// Only applies during early phases (before TrafficSwitching).
	// +kubebuilder:default=true
	OnJobFailure bool `json:"onJobFailure,omitempty"`
	// OnValidationFailure triggers rollback if pre-promotion hook fails.
	// +kubebuilder:default=true
	OnValidationFailure bool `json:"onValidationFailure,omitempty"`
	// OnTrafficFailure triggers rollback if Green fails health checks
	// during TrafficSwitching phase (stabilization period).
	// +kubebuilder:default=false
	OnTrafficFailure bool `json:"onTrafficFailure,omitempty"`
	// StabilizationSeconds is the observation period during TrafficSwitching.
	// Green must remain healthy for this duration before proceeding to DemotingBlue.
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=0
	// +optional
	StabilizationSeconds *int32 `json:"stabilizationSeconds,omitempty"`
}

// BlueGreenConfig configures the behavior when Type is BlueGreen.
type BlueGreenConfig struct {
	// AutoPromote controls whether the operator automatically switches traffic
	// and deletes the old cluster after sync. If false, it pauses at PhaseSynced
	// waiting for manual approval (annotation or field update).
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

	// TrafficStrategy controls how traffic is routed during blue/green upgrades.
	// When empty, the effective strategy defaults to ServiceSelectors when
	// Gateway is disabled, and GatewayWeights when spec.gateway.enabled is true.
	// +optional
	TrafficStrategy BlueGreenTrafficStrategy `json:"trafficStrategy,omitempty"`
}

// BlueGreenTrafficStrategy defines how traffic is routed during blue/green upgrades.
// +kubebuilder:validation:Enum=ServiceSelectors;GatewayWeights
type BlueGreenTrafficStrategy string

const (
	// BlueGreenTrafficStrategyServiceSelectors routes traffic by switching the
	// external Service selector between Blue and Green revisions. This is the
	// default strategy when Gateway API is not enabled for the cluster.
	BlueGreenTrafficStrategyServiceSelectors BlueGreenTrafficStrategy = "ServiceSelectors"
	// BlueGreenTrafficStrategyGatewayWeights routes traffic using Gateway API
	// weighted backends when spec.gateway.enabled is true. In this mode, the
	// external Gateway HTTPRoute is responsible for gradually shifting traffic
	// between Blue and Green.
	BlueGreenTrafficStrategyGatewayWeights BlueGreenTrafficStrategy = "GatewayWeights"
)

// UpdateStrategy controls how the operator handles version changes.
type UpdateStrategy struct {
	// Type controls how the operator handles version changes.
	// +kubebuilder:default="RollingUpdate"
	Type UpdateStrategyType `json:"type,omitempty"`

	// BlueGreen configures the behavior when Type is BlueGreen.
	// +optional
	BlueGreen *BlueGreenConfig `json:"blueGreen,omitempty"`
}

// UpgradeConfig defines configuration for upgrade operations.
type UpgradeConfig struct {
	// ExecutorImage is the container image to use for upgrade operations.
	//
	// This image is used by Kubernetes Jobs created during upgrades (for example, blue/green
	// cluster orchestration actions). The executor runs inside the tenant namespace and
	// authenticates to OpenBao using a projected ServiceAccount token (JWT auth).
	// +optional
	ExecutorImage string `json:"executorImage,omitempty"`

	// PreUpgradeSnapshot, when true, triggers a backup before any upgrade.
	// When enabled, the upgrade manager will create a backup using the backup
	// configuration (spec.backup.target, spec.backup.executorImage, etc.) and
	// wait for it to complete before proceeding with the upgrade.
	//
	// If the backup fails, the upgrade will be blocked and a Degraded condition
	// will be set with Reason=PreUpgradeBackupFailed.
	//
	// Requires spec.backup to be configured with target, executorImage, and
	// authentication (jwtAuthRole or tokenSecretRef).
	// +optional
	PreUpgradeSnapshot bool `json:"preUpgradeSnapshot,omitempty"`
	// JWTAuthRole is the name of the JWT Auth role configured in OpenBao
	// for upgrade operations. When set, the upgrade manager will use JWT Auth
	// (projected ServiceAccount token) instead of a static token. This is the preferred authentication
	// method as tokens are automatically rotated by Kubernetes.
	//
	// The role must be configured in OpenBao and must grant:
	// - "read" capability on sys/health
	// - "update" capability on sys/step-down
	// - "read" capability on sys/storage/raft/snapshot (if preUpgradeSnapshot is enabled)
	// The role must bind to the upgrade ServiceAccount (<cluster-name>-upgrade-serviceaccount),
	// which is automatically created by the operator.
	//
	// Either JWTAuthRole or TokenSecretRef must be set for upgrade operations.
	// +optional
	JWTAuthRole string `json:"jwtAuthRole,omitempty"`
	// TokenSecretRef optionally references a Secret containing an OpenBao API
	// token to use for upgrade operations.
	//
	// The token must have permission to:
	// - read sys/health
	// - update sys/step-down
	// - read sys/storage/raft/snapshot (if preUpgradeSnapshot is enabled)
	//
	// Either JWTAuthRole or TokenSecretRef must be set for upgrade operations.
	// If JWTAuthRole is set, this field is ignored in favor of JWT Auth.
	// +optional
	TokenSecretRef *corev1.SecretReference `json:"tokenSecretRef,omitempty"`
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
	// This is useful in restricted multi-tenant environments where the operator may not have
	// permissions to read Services/Endpoints in the default namespace.
	// Example: "10.43.0.0/16" for service network or "192.168.1.0/24" for control plane nodes.
	// +optional
	APIServerCIDR string `json:"apiServerCIDR,omitempty"`

	// APIServerEndpointIPs is an optional list of Kubernetes API server endpoint IPs.
	// When set, the operator adds least-privilege NetworkPolicy egress rules for these IPs on port 6443.
	// This is required on some CNI implementations where egress enforcement happens on the post-NAT
	// destination (the API server endpoint) rather than the kubernetes Service IP (10.43.0.1:443).
	//
	// Use this in restricted environments where the operator cannot list EndpointSlices/Endpoints in
	// the default namespace to auto-detect endpoint IPs.
	// Example (k3d): ["192.168.166.2"]
	// +optional
	APIServerEndpointIPs []string `json:"apiServerEndpointIPs,omitempty"`

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
}

// BackupTarget describes a generic, cloud-agnostic object storage destination.
type BackupTarget struct {
	// Endpoint is the HTTP(S) endpoint for the object storage service.
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// Bucket is the bucket or container name.
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`
	// Region is the AWS region to use for S3-compatible clients.
	// For AWS, this should match the bucket region (for example, "eu-west-1").
	// For many S3-compatible stores (MinIO/Ceph), this can be any non-empty value.
	// +optional
	// +kubebuilder:default=us-east-1
	Region string `json:"region,omitempty"`
	// PathPrefix is an optional prefix within the bucket for this cluster's snapshots.
	// +optional
	PathPrefix string `json:"pathPrefix,omitempty"`
	// RoleARN is the IAM role ARN (or S3-compatible equivalent) to assume via Web Identity.
	// When set, the backup Job mounts a projected ServiceAccount token and relies on the
	// cloud provider SDK default credential chain (for example, AWS IRSA).
	// +optional
	RoleARN string `json:"roleArn,omitempty"`
	// CredentialsSecretRef optionally references a Secret containing credentials for the object store.
	// +optional
	CredentialsSecretRef *corev1.SecretReference `json:"credentialsSecretRef,omitempty"`
	// UsePathStyle controls whether to use path-style addressing (bucket.s3.amazonaws.com/object)
	// or virtual-hosted-style addressing (bucket.s3.amazonaws.com/object).
	// Set to true for MinIO and S3-compatible stores that require path-style.
	// Set to false for AWS S3 (default, as AWS is deprecating path-style).
	// +optional
	// +kubebuilder:default=false
	UsePathStyle bool `json:"usePathStyle,omitempty"`
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
}

// SelfInitOperation defines valid operations for self-initialization requests.
// +kubebuilder:validation:Enum=create;read;update;delete;list
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
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`
	// BootstrapJWTAuth, when true, enables automatic JWT authentication bootstrap
	// for the operator. This configures JWT auth in OpenBao, sets up OIDC discovery,
	// creates the operator policy and role, and optionally creates backup/upgrade
	// policies and roles if those features are configured with JWTAuthRole.
	//
	// When false (default), JWT auth must be manually configured via SelfInit requests.
	// This is the recommended approach for users who want full control over JWT auth setup.
	//
	// Bootstrap requires:
	// - OIDC issuer URL to be discoverable at operator startup
	// - OIDC JWKS public keys to be available at operator startup
	// - Operator namespace and service account to be known
	//
	// +kubebuilder:default=false
	// +optional
	BootstrapJWTAuth bool `json:"bootstrapJWTAuth,omitempty"`
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
	// This value is required; the operator does not apply a default image.
	// When omitted or empty, validation will reject the OpenBaoCluster.
	// +kubebuilder:validation:MinLength=1
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
	// Address is the full address to the OpenBao cluster.
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
	// Address is the address of the KMIP server.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`

	// Certificate is the path to the client certificate for KMIP communication.
	// +optional
	Certificate string `json:"certificate,omitempty"`

	// Key is the path to the private key for KMIP communication.
	// +optional
	Key string `json:"key,omitempty"`

	// CACert is the path to the CA certificate for KMIP communication.
	// +optional
	CACert string `json:"caCert,omitempty"`

	// TLSServerName is the SNI host name to use when connecting via TLS.
	// +optional
	TLSServerName string `json:"tlsServerName,omitempty"`

	// TLSSkipVerify disables verification of TLS certificates.
	// +optional
	TLSSkipVerify *bool `json:"tlsSkipVerify,omitempty"`
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

	// AuthType is the authentication type (e.g., "instance_principal", "user_principal").
	// +optional
	AuthType string `json:"authType,omitempty"`

	// CompartmentID is the OCID of the compartment containing the key.
	// +optional
	CompartmentID string `json:"compartmentID,omitempty"`
}

// PKCS11SealConfig configures the PKCS#11 seal type.
// See: https://openbao.org/docs/configuration/seal/pkcs11/
type PKCS11SealConfig struct {
	// Lib is the path to the PKCS#11 library provided by the HSM vendor.
	// +kubebuilder:validation:MinLength=1
	Lib string `json:"lib"`

	// Slot is the slot number where the HSM token is located.
	// +optional
	Slot string `json:"slot,omitempty"`

	// PIN is the PIN for accessing the HSM token.
	// Note: It is strongly recommended to use CredentialsSecretRef instead of setting this directly.
	// +optional
	PIN string `json:"pin,omitempty"`

	// KeyLabel is the label for the encryption key used by OpenBao.
	// +kubebuilder:validation:MinLength=1
	KeyLabel string `json:"keyLabel"`

	// HMACKeyLabel is the label for the HMAC key used by OpenBao.
	// +optional
	HMACKeyLabel string `json:"hmacKeyLabel,omitempty"`

	// GenerateKey indicates whether OpenBao should generate the key if it doesn't exist.
	// +optional
	GenerateKey *bool `json:"generateKey,omitempty"`

	// RSAEncryptLocal allows performing encryption locally for HSMs that don't support encryption for RSA keys.
	// +optional
	RSAEncryptLocal *bool `json:"rsaEncryptLocal,omitempty"`

	// RSAOAEPHash specifies the hash algorithm to use for RSA with OAEP padding.
	// Valid values: sha1, sha224, sha256, sha384, sha512.
	// +kubebuilder:validation:Enum=sha1;sha224;sha256;sha384;sha512
	// +optional
	RSAOAEPHash string `json:"rsaOAEPHash,omitempty"`
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
	// (e.g., AWS_ACCESS_KEY_ID, GOOGLE_CREDENTIALS JSON, Azure client secret, etc.).
	// If using Workload Identity (IRSA, GKE WI, Azure MSI), this can be omitted.
	// +optional
	CredentialsSecretRef *corev1.SecretReference `json:"credentialsSecretRef,omitempty"`
}

// ImageVerificationConfig configures supply chain security checks for container images.
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
	// For official OpenBao images, use: https://token.actions.githubusercontent.com
	// +optional
	Issuer string `json:"issuer,omitempty"`

	// Subject is the OIDC subject for keyless verification.
	// Required for keyless verification when PublicKey is not provided.
	// For official OpenBao images, use: https://github.com/openbao/openbao/.github/workflows/release.yml@refs/tags/v<VERSION>
	// The version in the subject MUST match the image tag version.
	// +optional
	Subject string `json:"subject,omitempty"`

	// FailurePolicy defines behavior on verification failure.
	// "Block" prevents StatefulSet updates when verification fails.
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

// SentinelConfig configures the Sentinel sidecar controller that watches for infrastructure drift.
type SentinelConfig struct {
	// Enabled controls whether the Sentinel sidecar controller is deployed.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// Image allows overriding the Sentinel container image.
	// If not specified, defaults to "openbao/operator-sentinel:vX.Y.Z" (matching operator version).
	// +optional
	Image string `json:"image,omitempty"`

	// Resources allows configuring resource limits for the Sentinel.
	// If not specified, defaults to requests/limits: 64Mi memory, 100m CPU.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// DebounceWindowSeconds controls the debounce window for drift detection.
	// Multiple drift events within this window will be coalesced into a single trigger.
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	// +optional
	DebounceWindowSeconds *int32 `json:"debounceWindowSeconds,omitempty"`
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

	// DisableStandbyReads disables reads from standby nodes.
	// When true, all reads must go to the active node.
	// +optional
	DisableStandbyReads *bool `json:"disableStandbyReads,omitempty"`

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

	// DownloadBehavior specifies how plugins are downloaded.
	// +kubebuilder:validation:Enum=standard;direct
	// +optional
	DownloadBehavior string `json:"downloadBehavior,omitempty"`
}

// OpenBaoClusterSpec defines the desired state of an OpenBaoCluster.
// The Operator owns certain protected OpenBao configuration stanzas (for example,
// listener "tcp", storage "raft", and seal "static" when using default unseal).
// Users must not override these via spec.configuration.
// +kubebuilder:validation:XValidation:rule="self.tls.mode != 'OperatorManaged' || size(self.tls.rotationPeriod) > 0",message="spec.tls.rotationPeriod is required when spec.tls.mode is OperatorManaged"
type OpenBaoClusterSpec struct {
	// Version is the semantic OpenBao version, used for upgrade orchestration.
	// The Operator uses static auto-unseal, which requires OpenBao v2.4.0 or later.
	// Versions below 2.4.0 do not support the static seal feature and will fail to start.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
	// Image is the container image to run; defaults may be derived from Version.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Replicas is the desired number of OpenBao pods.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3
	Replicas int32 `json:"replicas"`
	// Paused, when true, pauses reconciliation for this OpenBaoCluster (except delete and finalizers).
	// +optional
	Paused bool `json:"paused,omitempty"`
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
	Audit []AuditDevice `json:"audit,omitempty"`
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
	// When spec.upgrade.preUpgradeSnapshot is true, if upgrade authentication is not
	// Upgrade authentication must be explicitly configured via spec.upgrade.jwtAuthRole
	// or spec.upgrade.tokenSecretRef. The operator automatically creates an upgrade ServiceAccount
	// (<cluster-name>-upgrade-serviceaccount) for JWT Auth authentication.
	// +optional
	Upgrade *UpgradeConfig `json:"upgrade,omitempty"`
	// Unseal defines the auto-unseal configuration.
	// If omitted, defaults to "static" mode managed by the operator.
	// +optional
	Unseal *UnsealConfig `json:"unseal,omitempty"`
	// ImageVerification configures supply chain security checks.
	// +optional
	ImageVerification *ImageVerificationConfig `json:"imageVerification,omitempty"`
	// WorkloadHardening configures opt-in workload hardening features.
	// +optional
	WorkloadHardening *WorkloadHardeningConfig `json:"workloadHardening,omitempty"`
	// Profile defines the security posture for this cluster.
	// When set to "Hardened", the operator enforces strict security requirements:
	// - TLS must be External (cert-manager/CSI managed)
	// - Unseal must use external KMS (no static unseal)
	// - SelfInit must be enabled (no root token)
	// When set to "Development", relaxed security is allowed but a security warning
	// condition is set.
	// +kubebuilder:validation:Enum=Hardened;Development
	// +optional
	Profile Profile `json:"profile,omitempty"`
	// Sentinel configures the high-availability watcher/healer.
	// +optional
	Sentinel *SentinelConfig `json:"sentinel,omitempty"`
	// UpdateStrategy controls how the operator handles version changes.
	// +optional
	UpdateStrategy UpdateStrategy `json:"updateStrategy,omitempty"`
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
}

// BlueGreenPhase is a high-level summary of blue/green upgrade state.
// +kubebuilder:validation:Enum=Idle;DeployingGreen;JoiningMesh;Syncing;Promoting;TrafficSwitching;DemotingBlue;Cleanup;RollingBack;RollbackCleanup
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
	// PhaseTrafficSwitching indicates traffic is being switched to Green before demoting Blue.
	// This includes an optional stabilization period to observe Green under traffic.
	PhaseTrafficSwitching BlueGreenPhase = "TrafficSwitching"
	// PhaseDemotingBlue indicates Blue nodes are being demoted to non-voters.
	PhaseDemotingBlue BlueGreenPhase = "DemotingBlue"
	// PhaseCleanup indicates Blue StatefulSet is being deleted.
	PhaseCleanup BlueGreenPhase = "Cleanup"
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
	// GreenRevision is the hash/name of the next cluster (if upgrade in progress).
	GreenRevision string `json:"greenRevision,omitempty"`
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
	// TrafficSwitchedTime records when traffic was switched to Green.
	// Used for calculating stabilization period.
	// +optional
	TrafficSwitchedTime *metav1.Time `json:"trafficSwitchedTime,omitempty"`

	// TrafficStep indicates the current traffic shifting step when using
	// GatewayWeights traffic strategy. It is used by the infra layer to
	// derive Gateway backend weights. The operator treats 0 as initial
	// state (100% Blue), 1 as canary (e.g., 90/10), 2 as mid-step (e.g., 50/50),
	// and 3 as final (0/100).
	// +optional
	TrafficStep int32 `json:"trafficStep,omitempty"`
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
	// LastFailureReason describes why the last backup failed (if applicable).
	// +optional
	LastFailureReason string `json:"lastFailureReason,omitempty"`
}

// DriftStatus tracks drift detection and correction events for a cluster.
type DriftStatus struct {
	// LastDriftDetected is the timestamp when drift was last detected by Sentinel.
	// +optional
	LastDriftDetected *metav1.Time `json:"lastDriftDetected,omitempty"`
	// DriftCorrectionCount is the total number of times drift has been detected and corrected.
	// +optional
	DriftCorrectionCount int32 `json:"driftCorrectionCount,omitempty"`
	// LastCorrectionTime is the timestamp when drift was last corrected by the operator.
	// +optional
	LastCorrectionTime *metav1.Time `json:"lastCorrectionTime,omitempty"`
	// LastDriftResource is the resource that triggered the last drift detection
	// (e.g., "Service/sentinel-cluster" or "StatefulSet/sentinel-cluster").
	// +optional
	LastDriftResource string `json:"lastDriftResource,omitempty"`
	// LastFullReconcileTime is the timestamp of the last full reconciliation
	// (including UpgradeManager and BackupManager). Used to prevent Sentinel-triggered
	// fast path from indefinitely blocking administrative operations.
	// +optional
	LastFullReconcileTime *metav1.Time `json:"lastFullReconcileTime,omitempty"`
	// ConsecutiveFastPaths tracks consecutive Sentinel-triggered reconciliations
	// without a full reconcile. Reset to 0 after each full reconcile.
	// +optional
	ConsecutiveFastPaths int32 `json:"consecutiveFastPaths,omitempty"`
}

// OpenBaoClusterStatus defines the observed state of an OpenBaoCluster.
type OpenBaoClusterStatus struct {
	// Phase is a high-level summary of the cluster state.
	// +optional
	Phase ClusterPhase `json:"phase,omitempty"`
	// ActiveLeader is the current Raft leader pod name, for example "prod-cluster-0".
	// +optional
	ActiveLeader string `json:"activeLeader,omitempty"`
	// ReadyReplicas is the number of replicas that are currently Ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
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
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
	// Upgrade tracks the state of an in-progress upgrade (if any).
	// When non-nil, an upgrade is in progress and the UpgradeManager is orchestrating
	// the pod-by-pod rolling update with leader step-down.
	// +optional
	Upgrade *UpgradeProgress `json:"upgrade,omitempty"`
	// Backup tracks the state of backups for this cluster.
	// +optional
	Backup *BackupStatus `json:"backup,omitempty"`
	// Drift tracks drift detection and correction events for this cluster.
	// +optional
	Drift *DriftStatus `json:"drift,omitempty"`
	// BlueGreen tracks the state of blue/green upgrades (if enabled).
	// +optional
	BlueGreen *BlueGreenStatus `json:"blueGreen,omitempty"`
	// OperationLock prevents concurrent long-running operations (upgrade/backup/restore)
	// from acting on the same cluster at the same time.
	// +optional
	OperationLock *OperationLockStatus `json:"operationLock,omitempty"`
	// BreakGlass records when the operator has halted quorum-risk automation and requires
	// explicit operator acknowledgment to continue.
	// +optional
	BreakGlass *BreakGlassStatus `json:"breakGlass,omitempty"`
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
// +kubebuilder:validation:Enum=RollbackConsensusRepairFailed
type BreakGlassReason string

const (
	BreakGlassReasonRollbackConsensusRepairFailed BreakGlassReason = "RollbackConsensusRepairFailed"
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
	// Options contains device-specific configuration options as a map.
	// This is a fallback for backward compatibility and advanced use cases.
	// If structured options (FileOptions, HTTPOptions, etc.) are provided, they take precedence.
	// The structure depends on the audit device type.
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
	// Headers without values will be ignored.
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
