package constants

import "time"

// Requeue intervals used by controllers.
const (
	RequeueShort    = 5 * time.Second
	RequeueStandard = 1 * time.Minute

	RequeueSafetyNetBase   = 20 * time.Minute
	RequeueSafetyNetJitter = 5 * time.Minute
)
