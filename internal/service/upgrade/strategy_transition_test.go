package upgrade

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func TestStrategyTransitionManager_AcceptsIdleTransitionsWithoutRenamingStableWorkload(t *testing.T) {
	tests := []struct {
		name               string
		accepted           openbaov1alpha1.UpdateStrategyType
		desired            openbaov1alpha1.UpdateStrategyType
		blueRevision       string
		statefulSetName    string
		controllerRevision string
	}{
		{
			name:               "rolling to bluegreen preserves unrevisioned workload",
			accepted:           openbaov1alpha1.UpdateStrategyRollingUpdate,
			desired:            openbaov1alpha1.UpdateStrategyBlueGreen,
			statefulSetName:    "bao",
			controllerRevision: "bao-6d89f76c4b",
		},
		{
			name:               "bluegreen to rolling preserves revisioned workload",
			accepted:           openbaov1alpha1.UpdateStrategyBlueGreen,
			desired:            openbaov1alpha1.UpdateStrategyRollingUpdate,
			blueRevision:       "blue123",
			statefulSetName:    "bao-blue123",
			controllerRevision: "bao-blue123-7f6b8c9d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := healthyStrategyTransitionCluster(tt.accepted, tt.desired, tt.blueRevision)
			sts := steadyStrategyStatefulSet(cluster, tt.statefulSetName, tt.blueRevision, tt.controllerRevision)
			if tt.accepted == openbaov1alpha1.UpdateStrategyBlueGreen {
				// A stable OnDelete workload can legitimately retain healthy pods on
				// currentRevision while its post-promotion template advances.
				sts.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType}
				sts.Status.UpdatedReplicas = 0
				sts.Status.UpdateRevision = tt.controllerRevision + "-desired"
			}
			objects := []client.Object{sts}
			for ordinal := int32(0); ordinal < cluster.Spec.Replicas; ordinal++ {
				objects = append(objects, readyStrategyPod(cluster, tt.statefulSetName, tt.blueRevision, tt.controllerRevision, ordinal))
			}

			scheme := runtime.NewScheme()
			if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
				t.Fatalf("add OpenBao scheme: %v", err)
			}
			if err := appsv1.AddToScheme(scheme); err != nil {
				t.Fatalf("add apps scheme: %v", err)
			}
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatalf("add core scheme: %v", err)
			}

			manager := NewStrategyTransitionManager(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build())
			result, err := manager.Reconcile(context.Background(), logr.Discard(), cluster)
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if result.RequeueAfter <= 0 {
				t.Fatalf("Reconcile() requeue = %s, want positive", result.RequeueAfter)
			}
			if cluster.Status.AcceptedUpgradeStrategy != tt.desired {
				t.Fatalf("accepted strategy = %q, want %q", cluster.Status.AcceptedUpgradeStrategy, tt.desired)
			}
			if got := StableVoterStatefulSetName(cluster); got != tt.statefulSetName {
				t.Fatalf("stable StatefulSet = %q, want %q", got, tt.statefulSetName)
			}
			if tt.desired == openbaov1alpha1.UpdateStrategyRollingUpdate && cluster.Status.BlueGreen.PreUpgradeSnapshotJobName != "preserved-history" {
				t.Fatalf("durable BlueGreen history was not preserved: %#v", cluster.Status.BlueGreen)
			}

			if tt.desired == openbaov1alpha1.UpdateStrategyBlueGreen {
				if cluster.Status.BlueGreen == nil {
					t.Fatal("BlueGreen status was not initialized")
				}
				if cluster.Status.BlueGreen.BlueRevision != "" {
					t.Fatalf("Blue revision = %q, want unrevisioned", cluster.Status.BlueGreen.BlueRevision)
				}
				if cluster.Status.BlueGreen.BlueControllerRevision != tt.controllerRevision {
					t.Fatalf("Blue controller revision = %q, want %q", cluster.Status.BlueGreen.BlueControllerRevision, tt.controllerRevision)
				}
			}
		})
	}
}

func TestStrategyTransitionManager_DefersWhileOperationActive(t *testing.T) {
	cluster := healthyStrategyTransitionCluster(
		openbaov1alpha1.UpdateStrategyRollingUpdate,
		openbaov1alpha1.UpdateStrategyBlueGreen,
		"",
	)
	cluster.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{
		Operation: openbaov1alpha1.ClusterOperationBackup,
		Holder:    "backup-manager",
	}

	manager := NewStrategyTransitionManager(fake.NewClientBuilder().Build())
	result, err := manager.Reconcile(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("Reconcile() requeue = %s, want zero so active manager can continue", result.RequeueAfter)
	}
	if cluster.Status.AcceptedUpgradeStrategy != openbaov1alpha1.UpdateStrategyRollingUpdate {
		t.Fatalf("accepted strategy changed during active operation: %q", cluster.Status.AcceptedUpgradeStrategy)
	}
}

func TestValidateIdleStrategyTransitionStatus_Blockers(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*openbaov1alpha1.OpenBaoCluster)
		wantErr string
	}{
		{name: "initializing", mutate: func(c *openbaov1alpha1.OpenBaoCluster) { c.Status.Initialized = false }, wantErr: "finish cluster initialization"},
		{name: "not running", mutate: func(c *openbaov1alpha1.OpenBaoCluster) { c.Status.Phase = openbaov1alpha1.ClusterPhaseUpgrading }, wantErr: "status.phase=Running"},
		{name: "version transition", mutate: func(c *openbaov1alpha1.OpenBaoCluster) { c.Status.CurrentVersion = "2.5.5" }, wantErr: "finish or recover the current version transition"},
		{name: "voters not ready", mutate: func(c *openbaov1alpha1.OpenBaoCluster) { c.Status.ReadyReplicas = 2 }, wantErr: "all voter replicas"},
		{name: "unavailable", mutate: func(c *openbaov1alpha1.OpenBaoCluster) { c.Status.Conditions = nil }, wantErr: "Available condition"},
		{name: "degraded", mutate: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.Conditions = append(c.Status.Conditions, metav1.Condition{Type: string(openbaov1alpha1.ConditionDegraded), Status: metav1.ConditionTrue})
		}, wantErr: "Degraded condition"},
		{name: "workload error", mutate: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.Workload = &openbaov1alpha1.WorkloadControllerStatus{LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "ApplyFailed"}}
		}, wantErr: "workload controller error"},
		{name: "rolling failure", mutate: func(c *openbaov1alpha1.OpenBaoCluster) { c.Status.Upgrade = &openbaov1alpha1.UpgradeProgress{} }, wantErr: "rolling upgrade"},
		{name: "operation lock", mutate: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{Operation: openbaov1alpha1.ClusterOperationRestore}
		}, wantErr: "operation lock"},
		{name: "safe mode", mutate: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.BreakGlass = &openbaov1alpha1.BreakGlassStatus{Active: true}
		}, wantErr: "safe mode"},
		{name: "bluegreen active", mutate: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{Phase: openbaov1alpha1.PhaseSyncing}
		}, wantErr: "blue/green phase"},
		{name: "green remains", mutate: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{Phase: openbaov1alpha1.PhaseIdle, GreenRevision: "green"}
		}, wantErr: "remove the Green revision"},
		{name: "job failure", mutate: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{Phase: openbaov1alpha1.PhaseIdle, JobFailureCount: 1}
		}, wantErr: "job failure state"},
		{name: "pending request", mutate: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Upgrade.Requests = &openbaov1alpha1.UpgradeRequestConfig{Retry: "retry-1"}
		}, wantErr: "pending spec.upgrade.requests"},
		{name: "read replica status missing", mutate: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 1}
		}, wantErr: "read replica status"},
		{name: "read replicas not ready", mutate: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 2}
			c.Status.ReadReplicas = &openbaov1alpha1.ReadReplicaStatus{ReadyReplicas: 1}
		}, wantErr: "read replicas to become ready"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := healthyStrategyTransitionCluster(
				openbaov1alpha1.UpdateStrategyRollingUpdate,
				openbaov1alpha1.UpdateStrategyBlueGreen,
				"",
			)
			tt.mutate(cluster)
			err := validateIdleStrategyTransitionStatus(cluster, openbaov1alpha1.UpdateStrategyBlueGreen)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateIdleStrategyTransitionStatus() error = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateIdleStrategyTransitionStatus_RetriesOwnBlockedCondition(t *testing.T) {
	cluster := healthyStrategyTransitionCluster(
		openbaov1alpha1.UpdateStrategyBlueGreen,
		openbaov1alpha1.UpdateStrategyRollingUpdate,
		"blue123",
	)
	cluster.Status.Conditions = append(cluster.Status.Conditions, metav1.Condition{
		Type:   string(openbaov1alpha1.ConditionDegraded),
		Status: metav1.ConditionTrue,
		Reason: ReasonUpgradeStrategyTransitionBlocked,
	})

	if err := validateIdleStrategyTransitionStatus(cluster, openbaov1alpha1.UpdateStrategyRollingUpdate); err != nil {
		t.Fatalf("validateIdleStrategyTransitionStatus() error = %v, want self-reported transition block to be retried", err)
	}
}

func TestNormalizeStrategyTransitionStatus_RefreshesHistoricalBlueBaseline(t *testing.T) {
	cluster := healthyStrategyTransitionCluster(
		openbaov1alpha1.UpdateStrategyRollingUpdate,
		openbaov1alpha1.UpdateStrategyBlueGreen,
		"",
	)
	cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
		BlueRevision:              "stale-revision",
		BlueControllerRevision:    "stale-controller-revision",
		BlueImage:                 "openbao/openbao:stale",
		PreUpgradeSnapshotJobName: "preserved-history",
	}
	sts := steadyStrategyStatefulSet(cluster, cluster.Name, "", "bao-current-controller-revision")

	normalizeStrategyTransitionStatus(cluster, openbaov1alpha1.UpdateStrategyBlueGreen, sts)

	if cluster.Status.BlueGreen.BlueRevision != "" {
		t.Fatalf("BlueRevision = %q, want unrevisioned rolling baseline", cluster.Status.BlueGreen.BlueRevision)
	}
	if cluster.Status.BlueGreen.BlueControllerRevision != "bao-current-controller-revision" {
		t.Fatalf("BlueControllerRevision = %q, want current controller revision", cluster.Status.BlueGreen.BlueControllerRevision)
	}
	if cluster.Status.BlueGreen.BlueImage != cluster.Spec.Image {
		t.Fatalf("BlueImage = %q, want %q", cluster.Status.BlueGreen.BlueImage, cluster.Spec.Image)
	}
	if cluster.Status.BlueGreen.PreUpgradeSnapshotJobName != "preserved-history" {
		t.Fatalf("durable history was not preserved: %#v", cluster.Status.BlueGreen)
	}
}

func TestStrategyTransitionManager_BlocksNonSteadyWorkload(t *testing.T) {
	cluster := healthyStrategyTransitionCluster(
		openbaov1alpha1.UpdateStrategyRollingUpdate,
		openbaov1alpha1.UpdateStrategyBlueGreen,
		"",
	)
	sts := steadyStrategyStatefulSet(cluster, cluster.Name, "", "bao-abc")
	sts.Status.ReadyReplicas = 2

	scheme := runtime.NewScheme()
	_ = openbaov1alpha1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	manager := NewStrategyTransitionManager(fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts).Build())

	_, err := manager.Reconcile(context.Background(), logr.Discard(), cluster)
	if err == nil {
		t.Fatal("Reconcile() error = nil, want blocked transition")
	}
	if reason, ok := operatorerrors.Reason(err); !ok || reason != ReasonUpgradeStrategyTransitionBlocked {
		t.Fatalf("Reconcile() reason = %q, %v; want %q", reason, ok, ReasonUpgradeStrategyTransitionBlocked)
	}
	if cluster.Status.AcceptedUpgradeStrategy != openbaov1alpha1.UpdateStrategyRollingUpdate {
		t.Fatalf("accepted strategy changed for non-steady workload: %q", cluster.Status.AcceptedUpgradeStrategy)
	}
}

func TestStrategyTransitionManager_BlocksOrphanedRevisionPod(t *testing.T) {
	cluster := healthyStrategyTransitionCluster(
		openbaov1alpha1.UpdateStrategyRollingUpdate,
		openbaov1alpha1.UpdateStrategyBlueGreen,
		"",
	)
	sts := steadyStrategyStatefulSet(cluster, cluster.Name, "", "bao-abc")
	objects := []client.Object{sts}
	for ordinal := int32(0); ordinal < cluster.Spec.Replicas-1; ordinal++ {
		objects = append(objects, readyStrategyPod(cluster, cluster.Name, "", "bao-abc", ordinal))
	}
	objects = append(objects, readyStrategyPod(cluster, "bao-orphan-revision", "orphan-revision", "bao-orphan", 0))

	scheme := runtime.NewScheme()
	_ = openbaov1alpha1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	manager := NewStrategyTransitionManager(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build())

	_, err := manager.Reconcile(context.Background(), logr.Discard(), cluster)
	if err == nil || !strings.Contains(err.Error(), "remove non-active voter pod bao-orphan-revision-0") {
		t.Fatalf("Reconcile() error = %v, want orphaned revision pod blocker", err)
	}
	if cluster.Status.AcceptedUpgradeStrategy != openbaov1alpha1.UpdateStrategyRollingUpdate {
		t.Fatalf("accepted strategy changed with orphaned revision pod: %q", cluster.Status.AcceptedUpgradeStrategy)
	}
}

func healthyStrategyTransitionCluster(
	accepted openbaov1alpha1.UpdateStrategyType,
	desired openbaov1alpha1.UpdateStrategyType,
	blueRevision string,
) *openbaov1alpha1.OpenBaoCluster {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "bao", Namespace: "tenant"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.6.0",
			Image:    "quay.io/openbao/openbao:2.6.0",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Strategy:    desired,
				JWTAuthRole: "upgrade-role",
				Image:       "ghcr.io/dc-tec/openbao-upgrade:test",
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:             true,
			Phase:                   openbaov1alpha1.ClusterPhaseRunning,
			CurrentVersion:          "2.6.0",
			ReadyReplicas:           3,
			AcceptedUpgradeStrategy: accepted,
			Conditions: []metav1.Condition{
				{Type: string(openbaov1alpha1.ConditionAvailable), Status: metav1.ConditionTrue},
			},
		},
	}
	if accepted == openbaov1alpha1.UpdateStrategyBlueGreen {
		cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
			Phase:                     openbaov1alpha1.PhaseIdle,
			BlueRevision:              blueRevision,
			BlueImage:                 cluster.Spec.Image,
			PreUpgradeSnapshotJobName: "preserved-history",
		}
	}
	return cluster
}

func steadyStrategyStatefulSet(
	cluster *openbaov1alpha1.OpenBaoCluster,
	name string,
	revision string,
	controllerRevision string,
) *appsv1.StatefulSet {
	labels := resourceidentity.Labels(cluster)
	labels[constants.LabelOpenBaoWorkloadPool] = constants.LabelValueOpenBaoWorkloadPoolVoter
	templateLabels := resourceidentity.VoterPodSelectorLabels(cluster)
	if revision != "" {
		templateLabels[constants.LabelOpenBaoRevision] = revision
	}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cluster.Namespace, Labels: labels, Generation: 1},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(cluster.Spec.Replicas),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: templateLabels},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: constants.ContainerBao, Image: cluster.Spec.Image}}},
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: ptr.To(int32(0)),
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           cluster.Spec.Replicas,
			CurrentReplicas:    cluster.Spec.Replicas,
			ReadyReplicas:      cluster.Spec.Replicas,
			UpdatedReplicas:    cluster.Spec.Replicas,
			CurrentRevision:    controllerRevision,
			UpdateRevision:     controllerRevision,
		},
	}
}

func readyStrategyPod(
	cluster *openbaov1alpha1.OpenBaoCluster,
	statefulSetName string,
	revision string,
	controllerRevision string,
	ordinal int32,
) *corev1.Pod {
	labels := resourceidentity.VoterPodSelectorLabels(cluster)
	labels[appsv1.ControllerRevisionHashLabelKey] = controllerRevision
	if revision != "" {
		labels[constants.LabelOpenBaoRevision] = revision
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", statefulSetName, ordinal),
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
}
