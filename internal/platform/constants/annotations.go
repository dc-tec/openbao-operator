package constants

// Annotation keys used by the operator.
const (
	// AnnotationTriggerBackup is the annotation key used to trigger an immediate manual backup.
	AnnotationTriggerBackup = "openbao.org/trigger-backup"
	// AnnotationConfigHash is the annotation key used to track ConfigMap/Secret changes.
	AnnotationConfigHash = "openbao.org/config-hash"
	// AnnotationMaintenance is the annotation key used to put a cluster into maintenance mode.
	AnnotationMaintenance = "openbao.org/maintenance"
	// AnnotationMaintenanceAllowed is the annotation key used to check if maintenance is allowed.
	AnnotationMaintenanceAllowed = "openbao.org/maintenance-allowed"
	// AnnotationRestartAt is the annotation key used to trigger a rolling restart via Pod template updates.
	AnnotationRestartAt = "openbao.org/restart-at"
	// AnnotationClaimUpgradeRequest records the claim-upgrade request that promoted claim service selectors.
	AnnotationClaimUpgradeRequest = "openbao.org/claim-upgrade-request"
	// AnnotationServiceOfferingRolloutUID records the rollout UID that created a claim-upgrade request.
	AnnotationServiceOfferingRolloutUID = "openbao.org/service-offering-rollout-uid"
)
