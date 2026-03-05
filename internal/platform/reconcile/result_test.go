package reconcile

import (
	"testing"
	"time"
)

func TestResult_RequeueContract(t *testing.T) {
	t.Parallel()

	var zero Result
	if zero.RequeueAfter != 0 {
		t.Fatalf("zero Result should mean no requeue, got %v", zero.RequeueAfter)
	}

	withRequeue := Result{RequeueAfter: 5 * time.Second}
	if withRequeue.RequeueAfter <= 0 {
		t.Fatalf("expected positive RequeueAfter for requeue contract, got %v", withRequeue.RequeueAfter)
	}
}
