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
	// AnnotationRestoreRevision records the OpenBaoRestore UID whose snapshot is
	// loaded by the Pod. Changing it triggers the required post-restore rollout.
	AnnotationRestoreRevision = "openbao.org/restore-revision"
	// AnnotationOpenBaoOwnerUID ties retained operator-managed resources to the owning OpenBaoCluster UID.
	AnnotationOpenBaoOwnerUID = "openbao.org/owner-uid"
)
