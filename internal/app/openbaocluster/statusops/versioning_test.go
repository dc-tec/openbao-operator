package statusops

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func TestObservedVersionFromPods_UsesLeaderWhenUnambiguous(t *testing.T) {
	pod0 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-0",
			Labels: map[string]string{
				portopenbao.LabelVersion: "2.0.0",
			},
		},
	}
	leader := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-1",
			Labels: map[string]string{
				portopenbao.LabelVersion: "2.1.0",
			},
		},
	}

	state := &StatusState{
		Pods:        []corev1.Pod{pod0, leader},
		Pod0:        &pod0,
		LeaderCount: 1,
		LeaderName:  "cluster-1",
	}

	got := ObservedVersionFromPods(state)
	assert.Equal(t, "2.1.0", got)
}

func TestObservedVersionFromPods_IgnoresLeaderVersionWhenAmbiguous(t *testing.T) {
	pod0 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-0",
			Labels: map[string]string{
				portopenbao.LabelVersion: "2.0.0",
			},
		},
	}
	leaderCandidate := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-1",
			Labels: map[string]string{
				portopenbao.LabelVersion: "2.1.0",
			},
		},
	}

	state := &StatusState{
		Pods:        []corev1.Pod{pod0, leaderCandidate},
		Pod0:        &pod0,
		LeaderCount: 2,
		LeaderName:  "cluster-1",
	}

	got := ObservedVersionFromPods(state)
	assert.Equal(t, "2.0.0", got)
}

func TestReconcileCurrentVersion_SkipsWhenRollingUpgradeStatusExists(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				TargetVersion:   "2.4.4",
				LastErrorReason: "UpgradeFailed",
			},
		},
	}

	state := &StatusState{
		RollingUpgradeInProgress: true,
		UpgradeInProgress:        false,
		UpgradeFailed:            true,
	}

	ReconcileCurrentVersion(logr.Discard(), cluster, state, "2.4.4")
	assert.Equal(t, "2.4.3", cluster.Status.CurrentVersion)
}

func TestReconcileCurrentVersion_DoesNotRegressWhenObservedVersionIsLower(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.4",
		},
	}

	state := &StatusState{
		RollingUpgradeInProgress: false,
		BlueGreenInProgress:      false,
		UpgradeInProgress:        false,
	}

	ReconcileCurrentVersion(logr.Discard(), cluster, state, "2.4.3")
	assert.Equal(t, "2.4.4", cluster.Status.CurrentVersion)
}

func TestReconcileCurrentVersion_AdvancesWhenObservedVersionIsHigher(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
		},
	}

	state := &StatusState{
		RollingUpgradeInProgress: false,
		BlueGreenInProgress:      false,
		UpgradeInProgress:        false,
	}

	ReconcileCurrentVersion(logr.Discard(), cluster, state, "2.4.4")
	assert.Equal(t, "2.4.4", cluster.Status.CurrentVersion)
}

func TestReconcileCurrentVersion_AdvancesAfterRollingUpgradeFinalization(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.4.4",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
			Upgrade:        nil, // rolling manager finalized by clearing upgrade status only
		},
	}

	state := &StatusState{
		RollingUpgradeInProgress: false,
		BlueGreenInProgress:      false,
		UpgradeInProgress:        false,
	}

	ReconcileCurrentVersion(logr.Discard(), cluster, state, "2.4.4")
	assert.Equal(t, "2.4.4", cluster.Status.CurrentVersion)
}

func TestMaybeAdvanceCurrentVersionForBlueGreen_AdvancesOnCompletion(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.3",
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:        openbaov1alpha1.PhaseIdle,
				BlueRevision: upgrade.OpenBaoClusterRevision("2.4.4", "openbao/openbao:2.4.4", 3),
			},
		},
	}

	MaybeAdvanceCurrentVersionForBlueGreen(logr.Discard(), cluster, "2.4.4")
	assert.Equal(t, "2.4.4", cluster.Status.CurrentVersion)
}
