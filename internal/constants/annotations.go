package constants

// Annotation keys used by the operator.
const (
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
	// AnnotationRestartAt is the annotation key used to trigger a rolling restart via Pod template updates.
	AnnotationRestartAt = "openbao.org/restart-at"

	// AnnotationLastDevelopmentWarning is the last time the operator emitted a Development profile warning event.
	AnnotationLastDevelopmentWarning = "openbao.org/last-development-warning"
	// AnnotationLastProfileNotSetWarning is the last time the operator emitted a missing profile warning event.
	AnnotationLastProfileNotSetWarning = "openbao.org/last-profile-not-set-warning"
	// AnnotationLastRootTokenWarning is the last time the operator emitted a root token storage warning event.
	// #nosec G101 -- This is a Kubernetes annotation key name, not a credential.
	AnnotationLastRootTokenWarning = "openbao.org/last-root-token-warning"
	// AnnotationLastStaticUnsealWarning is the last time the operator emitted a static unseal warning event.
	AnnotationLastStaticUnsealWarning = "openbao.org/last-static-unseal-warning"
)
