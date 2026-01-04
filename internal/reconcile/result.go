package reconcile

import "time"

// Result expresses whether reconciliation should be requeued, and after what delay.
// A zero RequeueAfter means "no requeue requested".
type Result struct {
	RequeueAfter time.Duration
}
