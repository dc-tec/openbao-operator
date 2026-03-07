package openbaocluster

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestContainsFinalizerAndRemoveFinalizer(t *testing.T) {
	t.Parallel()

	finalizers := []string{"alpha", openbaov1alpha1.OpenBaoClusterFinalizer, "beta", openbaov1alpha1.OpenBaoClusterFinalizer}
	if !containsFinalizer(finalizers, openbaov1alpha1.OpenBaoClusterFinalizer) {
		t.Fatal("expected containsFinalizer to find the requested finalizer")
	}
	if containsFinalizer(finalizers, "missing") {
		t.Fatal("containsFinalizer unexpectedly matched missing value")
	}

	got := removeFinalizer(finalizers, openbaov1alpha1.OpenBaoClusterFinalizer)
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("removeFinalizer() = %v, want [alpha beta]", got)
	}
}
