package upgrade

import (
	"strings"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/testutil/proptest"
	"pgregory.net/rapid"
)

func TestUpgradeRequestHelperProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		retry := requestValueGenerator().Draw(rt, "retry")
		promote := requestValueGenerator().Draw(rt, "promote")
		rollback := requestValueGenerator().Draw(rt, "rollback")
		lastRetry := requestValueGenerator().Draw(rt, "last_retry")
		lastPromote := requestValueGenerator().Draw(rt, "last_promote")
		lastRollback := requestValueGenerator().Draw(rt, "last_rollback")

		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Upgrade: &openbaov1alpha1.UpgradeConfig{
					Requests: &openbaov1alpha1.UpgradeRequestConfig{
						Retry:    retry,
						Promote:  promote,
						Rollback: rollback,
					},
				},
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				UpgradeRequests: &openbaov1alpha1.UpgradeRequestStatus{
					LastHandledRetry:    lastRetry,
					LastHandledPromote:  lastPromote,
					LastHandledRollback: lastRollback,
				},
			},
		}

		assertRequestHelper(t, "retry", RetryRequestValue(cluster), RetryRequestPending(cluster), retry, lastRetry)
		assertRequestHelper(t, "promote", PromoteRequestValue(cluster), PromoteRequestPending(cluster), promote, lastPromote)
		assertRequestHelper(t, "rollback", RollbackRequestValue(cluster), RollbackRequestPending(cluster), rollback, lastRollback)
	})
}

func TestUpgradeRequestMarkHandledProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		retry := requestValueGenerator().Draw(rt, "retry")
		promote := requestValueGenerator().Draw(rt, "promote")
		rollback := requestValueGenerator().Draw(rt, "rollback")
		status := &openbaov1alpha1.OpenBaoClusterStatus{}

		MarkRetryRequestHandled(status, retry)
		MarkPromoteRequestHandled(status, promote)
		MarkRollbackRequestHandled(status, rollback)

		if status.UpgradeRequests == nil {
			t.Fatalf("UpgradeRequests = nil, want initialized")
		}
		if got := status.UpgradeRequests.LastHandledRetry; got != strings.TrimSpace(retry) {
			t.Fatalf("LastHandledRetry = %q, want %q", got, strings.TrimSpace(retry))
		}
		if got := status.UpgradeRequests.LastHandledPromote; got != strings.TrimSpace(promote) {
			t.Fatalf("LastHandledPromote = %q, want %q", got, strings.TrimSpace(promote))
		}
		if got := status.UpgradeRequests.LastHandledRollback; got != strings.TrimSpace(rollback) {
			t.Fatalf("LastHandledRollback = %q, want %q", got, strings.TrimSpace(rollback))
		}

		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Upgrade: &openbaov1alpha1.UpgradeConfig{
					Requests: &openbaov1alpha1.UpgradeRequestConfig{
						Retry:    retry,
						Promote:  promote,
						Rollback: rollback,
					},
				},
			},
			Status: *status,
		}
		if RetryRequestPending(cluster) || PromoteRequestPending(cluster) || RollbackRequestPending(cluster) {
			t.Fatalf("requests remained pending after mark handled: status=%+v spec=%+v",
				cluster.Status.UpgradeRequests, cluster.Spec.Upgrade.Requests)
		}

		MarkRetryRequestHandled(nil, retry)
		MarkPromoteRequestHandled(nil, promote)
		MarkRollbackRequestHandled(nil, rollback)
	})
}

func TestUpgradeRequestNilSafeProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		cluster := &openbaov1alpha1.OpenBaoCluster{}
		if rapid.Bool().Draw(rt, "has_upgrade") {
			cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{}
		}
		if RetryRequestValue(cluster) != "" || PromoteRequestValue(cluster) != "" || RollbackRequestValue(cluster) != "" {
			t.Fatalf("request values = retry:%q promote:%q rollback:%q, want all empty",
				RetryRequestValue(cluster), PromoteRequestValue(cluster), RollbackRequestValue(cluster))
		}
		if RetryRequestPending(cluster) || PromoteRequestPending(cluster) || RollbackRequestPending(cluster) {
			t.Fatalf("requests are pending with missing request config")
		}
	})
}

func assertRequestHelper(t *testing.T, name, gotValue string, gotPending bool, rawRequest, rawHandled string) {
	t.Helper()

	wantValue := strings.TrimSpace(rawRequest)
	wantPending := wantValue != "" && wantValue != strings.TrimSpace(rawHandled)
	if gotValue != wantValue {
		t.Fatalf("%s request value = %q, want %q", name, gotValue, wantValue)
	}
	if gotPending != wantPending {
		t.Fatalf("%s request pending = %t, want %t for rawRequest=%q rawHandled=%q",
			name, gotPending, wantPending, rawRequest, rawHandled)
	}
}

func requestValueGenerator() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		prefix := whitespaceGenerator().Draw(t, "prefix")
		value := proptest.OptionalIdentifier().Draw(t, "value")
		suffix := whitespaceGenerator().Draw(t, "suffix")
		return prefix + value + suffix
	})
}

func whitespaceGenerator() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"", " ", "\t", "\n", " \t"})
}
