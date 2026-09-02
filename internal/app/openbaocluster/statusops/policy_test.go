package statusops

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestApplyPolicyProjectsObservedState(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Unix(1_700_000_000, 0))
	original := &openbaov1alpha1.OpenBaoCluster{
		Status: openbaov1alpha1.OpenBaoClusterStatus{ReadyReplicas: 1},
	}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Generation: 4},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			Version:  "2.4.4",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
		},
	}
	state := &StatusState{
		ReadyReplicas:    3,
		Available:        true,
		Initialized:      true,
		InitializedKnown: true,
		SealedKnown:      true,
		LeaderName:       "example-0",
		LeaderCount:      1,
		Pods: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "example-0",
				Labels: map[string]string{portopenbao.LabelVersion: "2.4.4"},
			},
		}},
	}

	result := ApplyPolicy(logr.Discard(), PolicyInput{
		Original: original,
		Cluster:  cluster,
		State:    state,
		Now:      now,
	})

	assert.Equal(t, int32(3), cluster.Status.ReadyReplicas)
	assert.Nil(t, cluster.Status.ReadReplicas)
	assert.Equal(t, "example-0", cluster.Status.ActiveLeader)
	assert.Equal(t, openbaov1alpha1.ClusterPhaseRunning, cluster.Status.Phase)
	assert.Equal(t, "2.4.4", cluster.Status.CurrentVersion)
	assert.Equal(t, constants.RequeueShort, result.RequeueAfter)

	available := findCondition(t, cluster, openbaov1alpha1.ConditionAvailable)
	assert.Equal(t, metav1.ConditionTrue, available.Status)
	assert.Equal(t, cluster.Generation, available.ObservedGeneration)
	assert.Equal(t, now, available.LastTransitionTime)
}

func TestApplyPolicyPreservesOtherControllerStatus(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{Replicas: 1},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			AcceptedUpgradeStrategy: openbaov1alpha1.UpdateStrategyRollingUpdate,
			Initialized:             true,
			SelfInitialized:         true,
			Upgrade:                 &openbaov1alpha1.UpgradeProgress{TargetVersion: "2.4.4"},
			UpgradeRequests:         &openbaov1alpha1.UpgradeRequestStatus{LastHandledRetry: "retry-1"},
			Backup:                  &openbaov1alpha1.BackupStatus{LastBackupName: "backup-1"},
			Restore:                 &openbaov1alpha1.ClusterRestoreStatus{Name: "restore-1"},
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:       openbaov1alpha1.PhaseDeployingGreen,
				OperationID: "upgrade-1",
			},
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation: openbaov1alpha1.ClusterOperationUpgrade,
				Holder:    "upgrade-1",
			},
			BreakGlass: &openbaov1alpha1.BreakGlassStatus{
				Active:  true,
				Reason:  openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
				Message: "repair required",
			},
			Workload: &openbaov1alpha1.WorkloadControllerStatus{
				LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "WorkloadFailed", Message: "workload detail"},
			},
			AdminOps: &openbaov1alpha1.AdminOpsControllerStatus{
				LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "AdminOpsFailed", Message: "admin detail"},
			},
		},
	}
	original := cluster.DeepCopy()
	statusBefore := cluster.DeepCopy().Status

	ApplyPolicy(logr.Discard(), PolicyInput{
		Original: original,
		Cluster:  cluster,
		State:    &StatusState{},
		Now:      metav1.Now(),
	})

	assert.Equal(t, statusBefore.AcceptedUpgradeStrategy, cluster.Status.AcceptedUpgradeStrategy)
	assert.Equal(t, statusBefore.Initialized, cluster.Status.Initialized)
	assert.Equal(t, statusBefore.SelfInitialized, cluster.Status.SelfInitialized)
	assert.Equal(t, statusBefore.Upgrade, cluster.Status.Upgrade)
	assert.Equal(t, statusBefore.UpgradeRequests, cluster.Status.UpgradeRequests)
	assert.Equal(t, statusBefore.Backup, cluster.Status.Backup)
	assert.Equal(t, statusBefore.Restore, cluster.Status.Restore)
	assert.Equal(t, statusBefore.BlueGreen, cluster.Status.BlueGreen)
	assert.Equal(t, statusBefore.OperationLock, cluster.Status.OperationLock)
	assert.Equal(t, statusBefore.BreakGlass, cluster.Status.BreakGlass)
	assert.Equal(t, statusBefore.Workload, cluster.Status.Workload)
	assert.Equal(t, statusBefore.AdminOps, cluster.Status.AdminOps)
}

func TestBuildReadReplicaStatusProjectsStorage(t *testing.T) {
	t.Parallel()

	configuredStorageClass := "configured"
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
				Replicas: 2,
				Storage: &openbaov1alpha1.ReadReplicaStorageConfig{
					StorageClassName: &configuredStorageClass,
				},
			},
		},
	}
	state := &StatusState{
		ReadReplicaReadyReplicas:            2,
		ReadReplicaRegisteredReplicas:       2,
		ReadReplicaHealthyReplicas:          1,
		ReadReplicaDataPVCCount:             2,
		ReadReplicaDataPVCStorageClassNames: []string{"observed"},
	}

	got := buildReadReplicaStatus(cluster, state)

	require.NotNil(t, got)
	assert.Equal(t, int32(2), got.DesiredReplicas)
	assert.Equal(t, int32(2), got.ReadyReplicas)
	assert.Equal(t, int32(2), got.RegisteredReplicas)
	assert.Equal(t, int32(1), got.HealthyReplicas)
	assert.Equal(t, int32(2), got.Storage.DesiredPVCs)
	assert.Equal(t, int32(2), got.Storage.BoundPVCs)
	assert.Equal(t, "observed", got.Storage.StorageClassName)
}

func findCondition(
	t *testing.T,
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
) metav1.Condition {
	t.Helper()

	for i := range cluster.Status.Conditions {
		if cluster.Status.Conditions[i].Type == string(conditionType) {
			return cluster.Status.Conditions[i]
		}
	}

	t.Fatalf("condition %q not found", conditionType)
	return metav1.Condition{}
}
