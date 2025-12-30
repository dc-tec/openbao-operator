package constants

// Annotation keys used by the operator and Sentinel.
const (
	// AnnotationSentinelTrigger is the annotation key used by the Sentinel to trigger reconciliation.
	// The Sentinel sets this annotation to a timestamp (RFC3339Nano) when it detects infrastructure drift.
	AnnotationSentinelTrigger = "openbao.org/sentinel-trigger"
	// AnnotationSentinelTriggerResource is the annotation key used by the Sentinel to record
	// which resource triggered the drift detection (e.g., "Service/sentinel-cluster").
	// This is set alongside AnnotationSentinelTrigger for observability.
	AnnotationSentinelTriggerResource = "openbao.org/sentinel-trigger-resource"
	// AnnotationTriggerBackup is the annotation key used to trigger an immediate manual backup.
	AnnotationTriggerBackup = "openbao.org/trigger-backup"
	// AnnotationConfigHash is the annotation key used to track ConfigMap/Secret changes.
	AnnotationConfigHash = "openbao.org/config-hash"
	// AnnotationForceRollback is a manual escape hatch annotation for blue/green upgrades.
	AnnotationForceRollback = "openbao.org/force-rollback"
	// AnnotationMaintenance is the annotation key used to put a cluster into maintenance mode.
	AnnotationMaintenance = "openbao.org/maintenance"
	// AnnotationMaintenanceAllowed is the annotation key used to check if maintenance is allowed.
	AnnotationMaintenanceAllowed = "openbao.org/maintenance-allowed"
)

// Sentinel resource names.
const (
	// SentinelServiceAccountName is the name of the ServiceAccount used by the Sentinel.
	SentinelServiceAccountName = "openbao-sentinel"
	// SentinelRoleName is the name of the Role created for the Sentinel.
	SentinelRoleName = "openbao-sentinel-role"
	// SentinelRoleBindingName is the name of the RoleBinding created for the Sentinel.
	SentinelRoleBindingName = "openbao-sentinel-rolebinding"
	// SentinelDeploymentNameSuffix is the suffix for the Sentinel Deployment name.
	SentinelDeploymentNameSuffix = "-sentinel"
)
