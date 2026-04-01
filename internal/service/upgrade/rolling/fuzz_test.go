package rolling

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

func FuzzRollingHelpers(f *testing.F) {
	f.Add("cluster-0", " retry ", "UpgradeFailed", "pod unhealthy")
	f.Add("weird", "", "", "")

	f.Fuzz(func(t *testing.T, podName, retryAnnotation, lastErrorReason, lastErrorMessage string) {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "default",
			},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Upgrade: &openbaov1alpha1.UpgradeConfig{
					Requests: &openbaov1alpha1.UpgradeRequestConfig{
						Retry: retryAnnotation,
					},
				},
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				Upgrade: &openbaov1alpha1.UpgradeProgress{
					LastErrorReason:  strings.TrimSpace(lastErrorReason),
					LastErrorMessage: strings.TrimSpace(lastErrorMessage),
					LastErrorAt:      &metav1.Time{Time: time.Unix(1_700_000_000, 0).UTC()},
					LastStepDownTime: &metav1.Time{Time: time.Unix(1_700_000_100, 0).UTC()},
				},
			},
		}

		_ = extractOrdinal(podName)
		_ = upgrade.RetryRequestValue(cluster)
		clearUpgradeFailureForRetry(cluster)
		if cluster.Status.Upgrade != nil {
			if cluster.Status.Upgrade.LastErrorReason != "" || cluster.Status.Upgrade.LastErrorMessage != "" {
				t.Fatalf("clearUpgradeFailureForRetry() should clear error fields")
			}
			if cluster.Status.Upgrade.LastErrorAt != nil || cluster.Status.Upgrade.LastStepDownTime != nil {
				t.Fatalf("clearUpgradeFailureForRetry() should clear timestamps")
			}
		}

		url := raftops.ClusterPodURLForService("default", "cluster-internal", sanitizeRollingName(podName, "cluster-0"))
		if !strings.HasPrefix(url, "https://") {
			t.Fatalf("ClusterPodURLForService() should return https URL")
		}
	})
}

func sanitizeRollingName(input, fallback string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(input, "\x00", ""))
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 40 {
		return trimmed[:40]
	}
	return trimmed
}
