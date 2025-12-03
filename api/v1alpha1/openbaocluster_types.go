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
)

// TLSConfig captures TLS configuration for an OpenBaoCluster.
type TLSConfig struct {
	// Enabled controls whether TLS is enabled for the cluster.
	// +kubebuilder:validation:Required
	Enabled bool `json:"enabled"`
	// RotationPeriod is a duration string (for example, "720h") controlling certificate rotation.
	// +kubebuilder:validation:MinLength=1
	RotationPeriod string `json:"rotationPeriod"`
	// ExtraSANs lists additional subject alternative names to include in server certificates.
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
	// KubernetesAuthRole is the name of the Kubernetes Auth role configured in OpenBao
	// for backup operations. When set, the backup executor will use Kubernetes Auth
	// (ServiceAccount token) instead of a static token. This is the preferred authentication
	// method as tokens are automatically rotated by Kubernetes.
	//
	// The role must be configured in OpenBao and must grant the "read" capability on
	// sys/storage/raft/snapshot. The role must bind to the backup ServiceAccount
	// (<cluster-name>-backup-serviceaccount) in the cluster namespace.
	// +optional
	KubernetesAuthRole string `json:"kubernetesAuthRole,omitempty"`
	// TokenSecretRef optionally references a Secret containing an OpenBao API
	// token to use for backup operations (fallback method).
	//
	// For standard clusters (non self-init), this is typically omitted and the
	// operator uses the root token from <cluster>-root-token. For self-init
	// clusters (no root token Secret), this field must reference a token with
	// permission to read sys/storage/raft/snapshot.
	//
	// If KubernetesAuthRole is set, this field is ignored in favor of Kubernetes Auth.
	// +optional
	TokenSecretRef *corev1.SecretReference `json:"tokenSecretRef,omitempty"`
	// Retention defines optional backup retention policy.
	// +optional
	Retention *BackupRetention `json:"retention,omitempty"`
	// PreUpgradeSnapshot, when true, triggers a backup before any upgrade.
	// +optional
	PreUpgradeSnapshot bool `json:"preUpgradeSnapshot,omitempty"`
	// ExecutorImage is the container image to use for backup operations.
	// Defaults to "openbao/backup-executor:v0.1.0" if not specified.
	// This allows users to override the image for air-gapped environments or custom registries.
	// +kubebuilder:validation:MinLength=1
	// +optional
	ExecutorImage string `json:"executorImage,omitempty"`
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

// OpenBaoClusterSpec defines the desired state of an OpenBaoCluster.
// The Operator owns certain protected OpenBao configuration stanzas (for example,
// listener "tcp", storage "raft", and seal "static"). Users must not override these
// via spec.config.
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
