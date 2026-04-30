package rolling

import (
	"context"
	"strconv"
	"strings"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/platform/testutil/openbao"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/backup"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
	"github.com/go-logr/logr/testr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	readReplicaTargetRevision = "rev-read-new"
	readReplicaOldRevision    = "rev-read-old"
)

func TestEnsureReadReplicaPoolReadyForRollingUpgrade_SkipsWhenReadReplicasAreNotConfigured(t *testing.T) {
	scheme := newScheme()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "upgrade-cluster", Namespace: "ns1"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newManagerWithClientFactory(c, scheme, backup.NewUpgradeStrategyRuntime(c, scheme), rollingTestClientFactory(), nil)

	result, waiting, err := mgr.ensureReadReplicaPoolReadyForRollingUpgrade(context.Background(), testr.New(t), cluster)
	if err != nil {
		t.Fatalf("ensureReadReplicaPoolReadyForRollingUpgrade() error = %v", err)
	}
	if waiting {
		t.Fatalf("waiting = true, want false")
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
}

func TestEnsureReadReplicaPoolReadyForRollingUpgrade_WaitsForStatefulSetConvergence(t *testing.T) {
	scheme := newScheme()
	cluster := readReplicaRollingTestCluster(2)
	readSTS := convergedReadReplicaStatefulSet(cluster)
	readSTS.Status.UpdatedReplicas = 1
	readSTS.Status.CurrentRevision = readReplicaOldRevision

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readSTS).Build()
	mgr := newManagerWithClientFactory(c, scheme, backup.NewUpgradeStrategyRuntime(c, scheme), rollingTestClientFactory(), nil)

	result, waiting, err := mgr.ensureReadReplicaPoolReadyForRollingUpgrade(context.Background(), testr.New(t), cluster)
	if err != nil {
		t.Fatalf("ensureReadReplicaPoolReadyForRollingUpgrade() error = %v", err)
	}
	if !waiting {
		t.Fatalf("waiting = false, want true")
	}
	if result.RequeueAfter != constants.RequeueShort {
		t.Fatalf("RequeueAfter = %v, want %v", result.RequeueAfter, constants.RequeueShort)
	}
}

func TestEnsureReadReplicaPoolReadyForRollingUpgrade_WaitsForPodHealth(t *testing.T) {
	scheme := newScheme()
	cluster := readReplicaRollingTestCluster(2)
	readSTS := convergedReadReplicaStatefulSet(cluster)
	pod0 := readyReadReplicaTestPod(cluster, 0)
	pod1 := readyReadReplicaTestPod(cluster, 1)
	caSecret := testClusterCASecret(cluster)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readSTS, pod0, pod1, caSecret).Build()
	mgr := newManagerWithClientFactory(c, scheme, backup.NewUpgradeStrategyRuntime(c, scheme), unhealthyReadReplicaClientFactory(), nil)

	result, waiting, err := mgr.ensureReadReplicaPoolReadyForRollingUpgrade(context.Background(), testr.New(t), cluster)
	if err != nil {
		t.Fatalf("ensureReadReplicaPoolReadyForRollingUpgrade() error = %v", err)
	}
	if !waiting {
		t.Fatalf("waiting = false, want true")
	}
	if result.RequeueAfter != constants.RequeueShort {
		t.Fatalf("RequeueAfter = %v, want %v", result.RequeueAfter, constants.RequeueShort)
	}
}

func TestEnsureReadReplicaPoolReadyForRollingUpgrade_SucceedsWhenReadPoolConverged(t *testing.T) {
	scheme := newScheme()
	cluster := readReplicaRollingTestCluster(2)
	readSTS := convergedReadReplicaStatefulSet(cluster)
	pod0 := readyReadReplicaTestPod(cluster, 0)
	pod1 := readyReadReplicaTestPod(cluster, 1)
	caSecret := testClusterCASecret(cluster)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readSTS, pod0, pod1, caSecret).Build()
	mgr := newManagerWithClientFactory(c, scheme, backup.NewUpgradeStrategyRuntime(c, scheme), rollingTestClientFactory(), nil)

	result, waiting, err := mgr.ensureReadReplicaPoolReadyForRollingUpgrade(context.Background(), testr.New(t), cluster)
	if err != nil {
		t.Fatalf("ensureReadReplicaPoolReadyForRollingUpgrade() error = %v", err)
	}
	if waiting {
		t.Fatalf("waiting = true, want false")
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
}

func readReplicaRollingTestCluster(replicas int32) *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "ns1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
				Replicas: replicas,
			},
		},
	}
}

func convergedReadReplicaStatefulSet(cluster *openbaov1alpha1.OpenBaoCluster) *appsv1.StatefulSet {
	replicas := cluster.Spec.ReadReplicas.Replicas
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       resourceidentity.ReadReplicaStatefulSetName(cluster),
			Namespace:  cluster.Namespace,
			Generation: 2,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2,
			ReadyReplicas:      replicas,
			UpdatedReplicas:    replicas,
			CurrentRevision:    readReplicaTargetRevision,
			UpdateRevision:     readReplicaTargetRevision,
		},
	}
}

func readyReadReplicaTestPod(cluster *openbaov1alpha1.OpenBaoCluster, ordinal int) *corev1.Pod {
	labels := resourceidentity.ReadReplicaPodSelectorLabels(cluster)
	labels[appsv1.StatefulSetRevisionLabel] = readReplicaTargetRevision
	labels["statefulset.kubernetes.io/pod-name"] = readReplicaPodName(cluster, ordinal)

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      readReplicaPodName(cluster, ordinal),
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
}

func readReplicaPodName(cluster *openbaov1alpha1.OpenBaoCluster, ordinal int) string {
	return resourceidentity.ReadReplicaStatefulSetName(cluster) + "-" + strconv.Itoa(ordinal)
}

func testClusterCASecret(cluster *openbaov1alpha1.OpenBaoCluster) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + constants.SuffixTLSCA,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("fake-ca"),
		},
	}
}

func unhealthyReadReplicaClientFactory() raftops.OpenBaoClientFactory {
	return func(config portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
		return &openbaoapi.MockClusterActions{
			IsHealthyFunc: func(ctx context.Context) (bool, error) {
				return !strings.Contains(config.BaseURL, "-read-1."), nil
			},
		}, nil
	}
}
