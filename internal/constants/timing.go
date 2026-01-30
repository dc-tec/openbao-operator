package constants

import (
	"os"
	"time"
)

// Requeue intervals used by controllers.
var (
	RequeueShort    = 5 * time.Second
	RequeueStandard = 1 * time.Minute

	RequeueSafetyNetBase   = 20 * time.Minute
	RequeueSafetyNetJitter = 5 * time.Minute

	SecurityWarningInterval = 1 * time.Hour

	ImageVerificationTimeout = 5 * time.Second
)

func init() {
	if val := os.Getenv("OPENBAO_REQUEUE_STANDARD"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			RequeueStandard = d
		}
	}
}
