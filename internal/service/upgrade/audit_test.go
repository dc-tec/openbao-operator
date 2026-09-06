package upgrade

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	testFromVersion = "2.4.4"
	testToVersion   = "2.5.0"
)

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
