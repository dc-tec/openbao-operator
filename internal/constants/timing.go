package constants

import "time"

// Requeue intervals used by controllers.
const (
	RequeueShort    = 5 * time.Second
	RequeueStandard = 1 * time.Minute

	RequeueSafetyNetBase   = 20 * time.Minute
	RequeueSafetyNetJitter = 5 * time.Minute

	SecurityWarningInterval = 1 * time.Hour
)

// Sentinel trigger rate limiting constants.
// These prevent Sentinel-triggered fast path from indefinitely blocking
// administrative operations like backups and upgrades (Sentinel DoS mitigation).
const (
	// SentinelMaxConsecutiveFastPaths is the maximum number of consecutive Sentinel-triggered
	// reconciliations before forcing a full reconcile (including Upgrade and Backup managers).
	SentinelMaxConsecutiveFastPaths int32 = 5

	// SentinelForceFullReconcileInterval is the minimum time between full reconciles.
	// If this much time has passed since the last full reconcile, the next reconcile
	// will include Upgrade and Backup managers regardless of Sentinel trigger.
	SentinelForceFullReconcileInterval = 5 * time.Minute
)
