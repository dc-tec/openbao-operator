package openbaocluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaolabels "github.com/dc-tec/openbao-operator/internal/openbao"
)

func TestObservedVersionFromPods_UsesLeaderWhenUnambiguous(t *testing.T) {
	pod0 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-0",
			Labels: map[string]string{
				openbaolabels.LabelVersion: "2.0.0",
			},
		},
	}
	leader := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-1",
			Labels: map[string]string{
				openbaolabels.LabelVersion: "2.1.0",
			},
		},
	}

	state := &clusterState{
		Pods:       []corev1.Pod{pod0, leader},
		Pod0:       &pod0,
		LeaderCount: 1,
		LeaderName:  "cluster-1",
	}

	got := observedVersionFromPods(state)
	assert.Equal(t, "2.1.0", got)
}

func TestObservedVersionFromPods_IgnoresLeaderVersionWhenAmbiguous(t *testing.T) {
	pod0 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-0",
			Labels: map[string]string{
				openbaolabels.LabelVersion: "2.0.0",
			},
		},
	}
	leaderCandidate := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-1",
			Labels: map[string]string{
				openbaolabels.LabelVersion: "2.1.0",
			},
		},
	}

	state := &clusterState{
		Pods:       []corev1.Pod{pod0, leaderCandidate},
		Pod0:       &pod0,
		LeaderCount: 2,
		LeaderName:  "cluster-1",
	}

	got := observedVersionFromPods(state)
	assert.Equal(t, "2.0.0", got)
}
