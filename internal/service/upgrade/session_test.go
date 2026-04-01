package upgrade

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestRootUpgradeSessionStartApply(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
		},
	}

	start := NewRootUpgradeSessionStart(cluster)
	if start.FromVersion != "2.4.4" {
		t.Fatalf("FromVersion=%q, want 2.4.4", start.FromVersion)
	}
	if start.ToVersion != "2.5.0" {
		t.Fatalf("ToVersion=%q, want 2.5.0", start.ToVersion)
	}
	if start.Replicas != 3 {
		t.Fatalf("Replicas=%d, want 3", start.Replicas)
	}

	start.Apply(&cluster.Status)
	if cluster.Status.Upgrade == nil {
		t.Fatal("expected upgrade status to be initialized")
	}
	if cluster.Status.Upgrade.FromVersion != "2.4.4" {
		t.Fatalf("status.Upgrade.FromVersion=%q, want 2.4.4", cluster.Status.Upgrade.FromVersion)
	}
	if cluster.Status.Upgrade.TargetVersion != "2.5.0" {
		t.Fatalf("status.Upgrade.TargetVersion=%q, want 2.5.0", cluster.Status.Upgrade.TargetVersion)
	}
	if cluster.Status.Upgrade.CurrentPartition != 3 {
		t.Fatalf("status.Upgrade.CurrentPartition=%d, want 3", cluster.Status.Upgrade.CurrentPartition)
	}
}

func TestCompleteRootUpgradeSession(t *testing.T) {
	t.Parallel()

	startedAt := metav1.NewTime(time.Now().Add(-3 * time.Minute))
	status := &openbaov1alpha1.OpenBaoClusterStatus{
		Upgrade: &openbaov1alpha1.UpgradeProgress{
			FromVersion: "2.4.4",
			StartedAt:   &startedAt,
		},
	}

	completion := CompleteRootUpgradeSession(status, "2.5.0", time.Now())
	if completion.FromVersion != "2.4.4" {
		t.Fatalf("completion.FromVersion=%q, want 2.4.4", completion.FromVersion)
	}
	if completion.ToVersion != "2.5.0" {
		t.Fatalf("completion.ToVersion=%q, want 2.5.0", completion.ToVersion)
	}
	if completion.Duration <= 0 {
		t.Fatalf("completion.Duration=%v, want > 0", completion.Duration)
	}
	if status.Upgrade != nil {
		t.Fatalf("status.Upgrade=%#v, want nil", status.Upgrade)
	}
	if status.CurrentVersion != "2.5.0" {
		t.Fatalf("status.CurrentVersion=%q, want 2.5.0", status.CurrentVersion)
	}
}

func TestUpgradeAuditFields(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "default",
		},
	}

	started := UpgradeStartedAuditFields(cluster, "rolling", "2.4.4", "2.5.0")
	if started["cluster_namespace"] != "default" {
		t.Fatalf("cluster_namespace=%q, want default", started["cluster_namespace"])
	}
	if started["cluster_name"] != "upgrade-cluster" {
		t.Fatalf("cluster_name=%q, want upgrade-cluster", started["cluster_name"])
	}
	if started["strategy"] != "rolling" {
		t.Fatalf("strategy=%q, want rolling", started["strategy"])
	}
	if started["from_version"] != "2.4.4" {
		t.Fatalf("from_version=%q, want 2.4.4", started["from_version"])
	}
	if started["to_version"] != "2.5.0" {
		t.Fatalf("to_version=%q, want 2.5.0", started["to_version"])
	}

	completed := UpgradeCompletedAuditFields(cluster, "bluegreen", "2.5.0")
	if completed["version"] != "2.5.0" {
		t.Fatalf("version=%q, want 2.5.0", completed["version"])
	}
	if completed["strategy"] != "bluegreen" {
		t.Fatalf("strategy=%q, want bluegreen", completed["strategy"])
	}
}
