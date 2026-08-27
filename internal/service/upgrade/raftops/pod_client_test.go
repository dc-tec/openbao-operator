package raftops

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

type stubClusterActions struct {
	isLeader bool
}

func (s stubClusterActions) IsSealed(context.Context) (bool, error)    { return false, nil }
func (s stubClusterActions) IsHealthy(context.Context) (bool, error)   { return true, nil }
func (s stubClusterActions) IsLeader(context.Context) (bool, error)    { return s.isLeader, nil }
func (s stubClusterActions) StepDownLeader(context.Context) error      { return nil }
func (s stubClusterActions) Snapshot(context.Context, io.Writer) error { return nil }
func (s stubClusterActions) LoginJWT(context.Context, string, string) (string, int, error) {
	return "", 0, nil
}
func (s stubClusterActions) Restore(context.Context, io.Reader, portopenbao.RestoreOptions) error {
	return nil
}

func TestNewClusterPodClient(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: "ns-a",
		},
	}

	var got portopenbao.ClientConfig
	client, err := NewClusterPodClient(
		cluster,
		"cluster-a-0",
		[]byte("ca-cert"),
		func(cfg portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
			got = cfg
			return stubClusterActions{}, nil
		},
		ClusterPodClientOptions{
			ConnectionTimeout:   2 * time.Second,
			RequestTimeout:      3 * time.Second,
			SmartClientDisabled: true,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "ns-a/cluster-a", got.ClusterKey)
	require.Equal(t, "https://cluster-a-0.cluster-a.ns-a.svc:8200", got.BaseURL)
	require.Equal(t, []byte("ca-cert"), got.CACert)
	require.Equal(t, 2*time.Second, got.ConnectionTimeout)
	require.Equal(t, 3*time.Second, got.RequestTimeout)
	require.True(t, got.SmartClientDisabled)
}

func TestFindClusterLeaderPodFallsBackToAPI(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: "ns-a",
		},
	}
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a-0"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a-1"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
	}

	leaderPod, source, ok := FindClusterLeaderPod(
		context.Background(),
		logr.Discard(),
		nil,
		func(cfg portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
			return stubClusterActions{isLeader: strings.Contains(cfg.BaseURL, "cluster-a-1")}, nil
		},
		cluster,
		pods,
		ClusterPodClientOptions{
			ConnectionTimeout:   2 * time.Second,
			RequestTimeout:      2 * time.Second,
			SmartClientDisabled: true,
		},
	)
	require.True(t, ok)
	require.Equal(t, "cluster-a-1", leaderPod)
	require.Equal(t, "api", source)
}
