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
			Version:  testToVersion,
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: testFromVersion,
		},
	}

	start := NewRootUpgradeSessionStart(cluster)
	if start.FromVersion != testFromVersion {
		t.Fatalf("FromVersion=%q, want %s", start.FromVersion, testFromVersion)
	}
	if start.ToVersion != testToVersion {
		t.Fatalf("ToVersion=%q, want %s", start.ToVersion, testToVersion)
	}
	if start.Replicas != 3 {
		t.Fatalf("Replicas=%d, want 3", start.Replicas)
	}

	start.Apply(&cluster.Status)
	if cluster.Status.Upgrade == nil {
		t.Fatal("expected upgrade status to be initialized")
	}
	if cluster.Status.Upgrade.FromVersion != testFromVersion {
		t.Fatalf("status.Upgrade.FromVersion=%q, want %s", cluster.Status.Upgrade.FromVersion, testFromVersion)
	}
	if cluster.Status.Upgrade.TargetVersion != testToVersion {
		t.Fatalf("status.Upgrade.TargetVersion=%q, want %s", cluster.Status.Upgrade.TargetVersion, testToVersion)
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
			FromVersion: testFromVersion,
			StartedAt:   &startedAt,
		},
	}

	completion := CompleteRootUpgradeSession(status, testToVersion, time.Now())
	if completion.FromVersion != testFromVersion {
		t.Fatalf("completion.FromVersion=%q, want %s", completion.FromVersion, testFromVersion)
	}
	if completion.ToVersion != testToVersion {
		t.Fatalf("completion.ToVersion=%q, want %s", completion.ToVersion, testToVersion)
	}
	if completion.Duration <= 0 {
		t.Fatalf("completion.Duration=%v, want > 0", completion.Duration)
	}
	if status.Upgrade != nil {
		t.Fatalf("status.Upgrade=%#v, want nil", status.Upgrade)
	}
	if status.CurrentVersion != testToVersion {
		t.Fatalf("status.CurrentVersion=%q, want %s", status.CurrentVersion, testToVersion)
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

	started := UpgradeStartedAuditFields(cluster, "rolling", testFromVersion, testToVersion)
	if started["cluster_namespace"] != "default" {
		t.Fatalf("cluster_namespace=%q, want default", started["cluster_namespace"])
	}
	if started["cluster_name"] != "upgrade-cluster" {
		t.Fatalf("cluster_name=%q, want upgrade-cluster", started["cluster_name"])
	}
	if started["strategy"] != "rolling" {
		t.Fatalf("strategy=%q, want rolling", started["strategy"])
	}
	if started["from_version"] != testFromVersion {
		t.Fatalf("from_version=%q, want %s", started["from_version"], testFromVersion)
	}
	if started["to_version"] != testToVersion {
		t.Fatalf("to_version=%q, want %s", started["to_version"], testToVersion)
	}

	completed := UpgradeCompletedAuditFields(cluster, "bluegreen", testToVersion)
	if completed["version"] != testToVersion {
		t.Fatalf("version=%q, want %s", completed["version"], testToVersion)
	}
	if completed["strategy"] != "bluegreen" {
		t.Fatalf("strategy=%q, want bluegreen", completed["strategy"])
	}
}
