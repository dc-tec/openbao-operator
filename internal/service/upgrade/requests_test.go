package upgrade

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestUpgradeRequestHelpers(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Requests: &openbaov1alpha1.UpgradeRequestConfig{
					Retry:    " retry-1 ",
					Promote:  " promote-1 ",
					Rollback: " rollback-1 ",
				},
			},
		},
	}

	if got := RetryRequestValue(cluster); got != "retry-1" {
		t.Fatalf("RetryRequestValue() = %q, want retry-1", got)
	}
	if got := PromoteRequestValue(cluster); got != "promote-1" {
		t.Fatalf("PromoteRequestValue() = %q, want promote-1", got)
	}
	if got := RollbackRequestValue(cluster); got != "rollback-1" {
		t.Fatalf("RollbackRequestValue() = %q, want rollback-1", got)
	}
	if !RetryRequestPending(cluster) || !PromoteRequestPending(cluster) || !RollbackRequestPending(cluster) {
		t.Fatal("expected all requests to be pending before marking them handled")
	}

	MarkRetryRequestHandled(&cluster.Status, "retry-1")
	MarkPromoteRequestHandled(&cluster.Status, "promote-1")
	MarkRollbackRequestHandled(&cluster.Status, "rollback-1")

	if RetryRequestPending(cluster) {
		t.Fatal("expected retry request to be handled")
	}
	if PromoteRequestPending(cluster) {
		t.Fatal("expected promote request to be handled")
	}
	if RollbackRequestPending(cluster) {
		t.Fatal("expected rollback request to be handled")
	}
}

func TestUpgradeRequestHelpers_NilSafe(t *testing.T) {
	t.Parallel()

	if RetryRequestValue(nil) != "" || PromoteRequestValue(nil) != "" || RollbackRequestValue(nil) != "" {
		t.Fatal("expected nil cluster request values to be empty")
	}
	if RetryRequestPending(nil) || PromoteRequestPending(nil) || RollbackRequestPending(nil) {
		t.Fatal("expected nil cluster requests to be non-pending")
	}

	status := &openbaov1alpha1.OpenBaoClusterStatus{}
	MarkRetryRequestHandled(status, "retry")
	MarkPromoteRequestHandled(status, "promote")
	MarkRollbackRequestHandled(status, "rollback")
	if status.UpgradeRequests == nil {
		t.Fatal("expected UpgradeRequests status to be initialized")
	}
}
