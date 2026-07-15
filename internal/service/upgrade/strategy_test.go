package upgrade

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestEffectiveStrategy(t *testing.T) {
	tests := []struct {
		name      string
		cluster   *openbaov1alpha1.OpenBaoCluster
		desired   openbaov1alpha1.UpdateStrategyType
		effective openbaov1alpha1.UpdateStrategyType
	}{
		{
			name:      "nil cluster defaults to rolling",
			desired:   openbaov1alpha1.UpdateStrategyRollingUpdate,
			effective: openbaov1alpha1.UpdateStrategyRollingUpdate,
		},
		{
			name: "spec strategy is effective before status initialization",
			cluster: &openbaov1alpha1.OpenBaoCluster{Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Upgrade: &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyBlueGreen},
			}},
			desired:   openbaov1alpha1.UpdateStrategyBlueGreen,
			effective: openbaov1alpha1.UpdateStrategyBlueGreen,
		},
		{
			name: "accepted strategy remains effective while transition is pending",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Upgrade: &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyBlueGreen},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					AcceptedUpgradeStrategy: openbaov1alpha1.UpdateStrategyRollingUpdate,
				},
			},
			desired:   openbaov1alpha1.UpdateStrategyBlueGreen,
			effective: openbaov1alpha1.UpdateStrategyRollingUpdate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DesiredStrategy(tt.cluster); got != tt.desired {
				t.Fatalf("DesiredStrategy() = %q, want %q", got, tt.desired)
			}
			if got := EffectiveStrategy(tt.cluster); got != tt.effective {
				t.Fatalf("EffectiveStrategy() = %q, want %q", got, tt.effective)
			}
		})
	}
}

func TestStableVoterWorkloadIdentity(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "bao"},
	}

	if got := StableVoterStatefulSetName(cluster); got != "bao" {
		t.Fatalf("StableVoterStatefulSetName() = %q, want bao", got)
	}
	if got := StableVoterPodName(cluster, 2); got != "bao-2" {
		t.Fatalf("StableVoterPodName() = %q, want bao-2", got)
	}

	cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{BlueRevision: "blue123"}
	if got := StableVoterStatefulSetName(cluster); got != "bao-blue123" {
		t.Fatalf("StableVoterStatefulSetName() = %q, want bao-blue123", got)
	}
	if got := StableVoterPodName(cluster, 1); got != "bao-blue123-1" {
		t.Fatalf("StableVoterPodName() = %q, want bao-blue123-1", got)
	}
}
