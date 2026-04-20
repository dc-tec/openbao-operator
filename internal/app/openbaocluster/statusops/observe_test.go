package statusops

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestGatherState_CollectsDataPVCStorageClasses(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	className := "gp3"
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
	}
	dataPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-example-0",
			Namespace: "default",
			Labels: map[string]string{
				"openbao.org/cluster": "example",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &className,
		},
	}
	otherPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scratch-volume",
			Namespace: "default",
			Labels: map[string]string{
				"openbao.org/cluster": "example",
			},
		},
	}

	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, dataPVC, otherPVC).Build()
	state, err := GatherState(context.Background(), logr.Discard(), reader, nil, nil, cluster, LabelConfig{
		AppInstanceKey:       "app.kubernetes.io/instance",
		AppManagedByKey:      "app.kubernetes.io/managed-by",
		AppManagedByValue:    "openbao-operator",
		OpenBaoClusterKey:    "openbao.org/cluster",
		OpenBaoComponentKey:  "openbao.org/component",
		BackupComponentValue: "backup",
		AppNameKey:           "app.kubernetes.io/name",
		AppNameValue:         "openbao",
		OpenBaoRevisionKey:   "openbao.org/revision",
	})
	require.NoError(t, err)
	require.Equal(t, 1, state.DataPVCCount)
	require.Equal(t, []string{"gp3"}, state.DataPVCStorageClassNames)
	require.False(t, state.DataPVCStorageClassUnset)
}

func TestGatherState_ObservesReadServingAndMembership(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
				Replicas: 2,
			},
		},
	}

	readStatefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-read",
			Namespace: "default",
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 2,
		},
	}
	voterStatefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 3,
		},
	}

	readPod0 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-read-0",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/instance":   "example",
				"app.kubernetes.io/name":       "openbao",
				"app.kubernetes.io/managed-by": "openbao-operator",
				"openbao.org/cluster":          "example",
				"openbao.org/workload-pool":    constants.LabelValueOpenBaoWorkloadPoolReadReplica,
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
	readPod1 := readPod0.DeepCopy()
	readPod1.Name = "example-read-1"

	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, voterStatefulSet, readStatefulSet, readPod0, readPod1).Build()
	factory := func(_ context.Context, _ *openbaov1alpha1.OpenBaoCluster, podName string) (PodObserver, error) {
		return fakePodObserver{
			health: map[string]*portopenbao.HealthStatus{
				"example-read-0": {Initialized: true, Sealed: false, Standby: true},
				"example-read-1": {Initialized: true, Sealed: false, Standby: true},
			},
			podName: podName,
		}, nil
	}
	membership := fakeMembershipRuntime{
		config: &portopenbao.RaftConfigurationResponse{
			Config: portopenbao.RaftConfiguration{
				Servers: []portopenbao.RaftServer{
					{NodeID: "example-0", Voter: true},
					{NodeID: "example-read-0", Voter: false},
					{NodeID: "example-read-1", Voter: false},
				},
			},
		},
		autopilot: &portopenbao.RaftAutopilotStateResponse{
			Servers: map[string]portopenbao.RaftAutopilotServerState{
				"example-read-0": {ID: "example-read-0", Healthy: true},
				"example-read-1": {ID: "example-read-1", Healthy: false},
			},
		},
	}

	state, err := GatherState(context.Background(), logr.Discard(), reader, factory, membership, cluster, LabelConfig{
		AppInstanceKey:       "app.kubernetes.io/instance",
		AppManagedByKey:      "app.kubernetes.io/managed-by",
		AppManagedByValue:    "openbao-operator",
		OpenBaoClusterKey:    "openbao.org/cluster",
		OpenBaoComponentKey:  "openbao.org/component",
		BackupComponentValue: "backup",
		AppNameKey:           "app.kubernetes.io/name",
		AppNameValue:         "openbao",
		OpenBaoRevisionKey:   "openbao.org/revision",
	})
	require.NoError(t, err)
	require.True(t, state.ReadServingKnown)
	require.True(t, state.ReadServingAvailable)
	require.True(t, state.ReadReplicaMembershipKnown)
	require.EqualValues(t, 2, state.ReadReplicaRegisteredReplicas)
	require.True(t, state.ReadReplicaAutopilotKnown)
	require.EqualValues(t, 1, state.ReadReplicaHealthyReplicas)
}

type fakePodObserver struct {
	health  map[string]*portopenbao.HealthStatus
	podName string
}

func (f fakePodObserver) Health(context.Context) (*portopenbao.HealthStatus, error) {
	health, ok := f.health[f.podName]
	if !ok {
		return nil, fmt.Errorf("missing health for pod %s", f.podName)
	}
	return health, nil
}

type fakeMembershipRuntime struct {
	config    *portopenbao.RaftConfigurationResponse
	autopilot *portopenbao.RaftAutopilotStateResponse
	err       error
}

func (f fakeMembershipRuntime) ReadRaftConfiguration(context.Context, logr.Logger, *openbaov1alpha1.OpenBaoCluster) (*portopenbao.RaftConfigurationResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.config, nil
}

func (f fakeMembershipRuntime) ReadRaftAutopilotState(context.Context, logr.Logger, *openbaov1alpha1.OpenBaoCluster) (*portopenbao.RaftAutopilotStateResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.autopilot, nil
}
