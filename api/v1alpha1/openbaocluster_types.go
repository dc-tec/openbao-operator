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
	RotationPeriod string `json:"rotationPeriod"`
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

// UpgradeConfig defines configuration for upgrade operations.
type UpgradeConfig struct {
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
}

// BackupTarget describes a generic, cloud-agnostic object storage destination.
type BackupTarget struct {
	// Endpoint is the HTTP(S) endpoint for the object storage service.
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// Bucket is the bucket or container name.
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`
	// PathPrefix is an optional prefix within the bucket for this cluster's snapshots.
	// +optional
	PathPrefix string `json:"pathPrefix,omitempty"`
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
	Operation SelfInitOperation `json:"operation"`
	// Path is the API path to call (e.g., "sys/audit/stdout", "auth/kubernetes/config").
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// Data contains the request payload as a structured map.
	// This must be a JSON/YAML object whose shape matches the target API
	// endpoint. Nested maps and lists are supported and are rendered into
	// the initialize stanza as HCL objects (for example, the "options" map
	// used by audit devices).
	//
	// This payload is stored in the OpenBaoCluster resource and persisted
	// in etcd; it must not contain sensitive values such as tokens,
	// passwords, or unseal keys.
	// +optional
	Data *apiextensionsv1.JSON `json:"data,omitempty"`
	// AllowFailure allows this request to fail without blocking initialization.
	// Defaults to false.
	// +optional
	AllowFailure bool `json:"allowFailure,omitempty"`
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

// UnsealConfig defines the auto-unseal configuration for an OpenBaoCluster.
// If omitted, defaults to "static" mode managed by the operator.
type UnsealConfig struct {
	// Type specifies the seal type: "static", "awskms", "gcpckms", "azurekeyvault", "transit".
	// Defaults to "static".
	// +kubebuilder:validation:Enum=static;awskms;gcpckms;azurekeyvault;transit
	// +kubebuilder:default=static
	Type string `json:"type,omitempty"`

	// Options allows specifying seal-specific configuration parameters.
	// These are rendered as attributes in the seal block of config.hcl.
	// +optional
	Options map[string]string `json:"options,omitempty"`

	// CredentialsSecretRef references a Secret containing provider credentials
	// (e.g., AWS_ACCESS_KEY_ID, GOOGLE_CREDENTIALS JSON).
	// If using Workload Identity (IRSA, GKE WI), this can be omitted.
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

// OpenBaoClusterSpec defines the desired state of an OpenBaoCluster.
// The Operator owns certain protected OpenBao configuration stanzas (for example,
// listener "tcp", storage "raft", and seal "static" when using default unseal).
// Users must not override these via spec.config.
// +kubebuilder:validation:XValidation:rule="!has(self.config) || self.config.all(k, v, !(k == 'listener' || k == 'storage' || k == 'seal'))",message="spec.config may not contain protected stanzas: listener, storage, seal"
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
	// Config contains optional user-supplied OpenBao configuration fragments.
	// Protected stanzas such as listener "tcp" and storage "raft" remain operator-owned.
	// +optional
	Config map[string]string `json:"config,omitempty"`
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
	// Profile defines the security posture for this cluster.
	// When set to "Hardened", the operator enforces strict security requirements:
	// - TLS must be External (cert-manager/CSI managed)
	// - Unseal must use external KMS (no static unseal)
	// - SelfInit must be enabled (no root token)
	// When set to "Development", relaxed security is allowed but a security warning
	// condition is set.
	// +kubebuilder:validation:Enum=Hardened;Development
	// +kubebuilder:default=Development
	// +optional
	Profile Profile `json:"profile,omitempty"`
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
	// Conditions represent the current state of the OpenBaoCluster resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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
	// Type is the type of audit device (e.g., "file", "syslog", "socket").
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
	// Path is the path of the audit device in the root namespace.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// Description is an optional description for the audit device.
	// +optional
	Description string `json:"description,omitempty"`
	// Options contains device-specific configuration options as a map.
	// The structure depends on the audit device type.
	// +optional
	Options *apiextensionsv1.JSON `json:"options,omitempty"`
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
