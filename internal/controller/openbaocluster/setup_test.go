package openbaocluster

import (
	"testing"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

func TestControllerRateLimitersKeepIndependentFailureCounts(t *testing.T) {
	t.Parallel()

	first := newControllerRateLimiter()
	second := newControllerRateLimiter()
	request := ctrl.Request{}

	if got := first.When(request); got != time.Second {
		t.Fatalf("first limiter initial delay = %s, want %s", got, time.Second)
	}
	if got := first.When(request); got != 2*time.Second {
		t.Fatalf("first limiter second delay = %s, want %s", got, 2*time.Second)
	}
	if got := second.When(request); got != time.Second {
		t.Fatalf("second limiter initial delay = %s, want %s", got, time.Second)
	}

	first.Forget(request)
	if got := second.When(request); got != 2*time.Second {
		t.Fatalf("second limiter delay after first limiter Forget = %s, want %s", got, 2*time.Second)
	}
}
