//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/infra"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/security"
	"github.com/dc-tec/openbao-operator/internal/upgrade/bluegreen"
)

func TestBlueGreenManager_CreatesJobsAndAdvancesPhases(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "bluegreen")
	cluster.Spec.UpdateStrategy = openbaov1alpha1.UpdateStrategy{
		Type: openbaov1alpha1.UpdateStrategyBlueGreen,
	}
	cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{
		ExecutorImage: "openbao/upgrade-executor:dev",
		JWTAuthRole:   "upgrade",
	}
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	createTLSSecret(t, namespace, cluster.Name)

	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
		status.CurrentVersion = "2.4.3"
		status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
			Phase:         openbaov1alpha1.PhaseJoiningMesh,
			BlueRevision:  "blue123",
			GreenRevision: "green456",
			StartTime:     &metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
		}
	})

	greenLeaderPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bluegreen-green456-0",
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppInstance:     cluster.Name,
				constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
				constants.LabelOpenBaoRevision: "green456",
				openbaoapi.LabelActive:         "true",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "noop",
					Image:   "busybox:1.36",
					Command: []string{"sh", "-c", "true"},
				},
			},
		},
	}
	if err := k8sClient.Create(ctx, greenLeaderPod); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	infraMgr := infra.NewManager(k8sClient, k8sScheme, "openbao-operator-system", "", nil, "")
	manager := bluegreen.NewManager(k8sClient, k8sScheme, infraMgr, openbaoapi.ClientConfig{}, security.NewImageVerifier(logr.Discard(), k8sClient, nil), security.NewImageVerifier(logr.Discard(), k8sClient, nil), "")

	// Phase: JoiningMesh -> create join job
	latestCluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, latestCluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}

	result, err := manager.Reconcile(ctx, logr.Discard(), latestCluster)
	if err != nil {
		t.Fatalf("reconcile JoiningMesh: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected requeue")
	}
	joinJob := findUpgradeJobByAction(t, namespace, string(bluegreen.ActionJoinGreenNonVoters))
	markJobSucceeded(t, joinJob)

	// Advance to Syncing
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, latestCluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	result, err = manager.Reconcile(ctx, logr.Discard(), latestCluster)
	if err != nil {
		t.Fatalf("reconcile after join success: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected requeue")
	}
	if latestCluster.Status.BlueGreen == nil || latestCluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseSyncing {
		t.Fatalf("phase=%v want=%v", phaseOrEmpty(latestCluster), openbaov1alpha1.PhaseSyncing)
	}
	if err := k8sClient.Status().Update(ctx, latestCluster); err != nil {
		t.Fatalf("persist cluster status: %v", err)
	}

	// Phase: Syncing -> create wait sync job
	result, err = manager.Reconcile(ctx, logr.Discard(), latestCluster)
	if err != nil {
		t.Fatalf("reconcile Syncing: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected requeue")
	}
	syncJob := findUpgradeJobByAction(t, namespace, string(bluegreen.ActionWaitGreenSynced))
	markJobSucceeded(t, syncJob)

	// Advance to Promoting
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, latestCluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	result, err = manager.Reconcile(ctx, logr.Discard(), latestCluster)
	if err != nil {
		t.Fatalf("reconcile after sync success: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected requeue")
	}
	if latestCluster.Status.BlueGreen == nil || latestCluster.Status.BlueGreen.Phase != openbaov1alpha1.PhasePromoting {
		t.Fatalf("phase=%v want=%v", phaseOrEmpty(latestCluster), openbaov1alpha1.PhasePromoting)
	}
	if err := k8sClient.Status().Update(ctx, latestCluster); err != nil {
		t.Fatalf("persist cluster status: %v", err)
	}

	// Phase: Promoting -> create promote job
	result, err = manager.Reconcile(ctx, logr.Discard(), latestCluster)
	if err != nil {
		t.Fatalf("reconcile Promoting: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected requeue")
	}
	promoteJob := findUpgradeJobByAction(t, namespace, string(bluegreen.ActionPromoteGreenVoters))
	markJobSucceeded(t, promoteJob)

	// Advance to DemotingBlue (cutover)
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, latestCluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	result, err = manager.Reconcile(ctx, logr.Discard(), latestCluster)
	if err != nil {
		t.Fatalf("reconcile after promote success: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected requeue")
	}
	if latestCluster.Status.BlueGreen == nil || latestCluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseDemotingBlue {
		t.Fatalf("phase=%v want=%v", phaseOrEmpty(latestCluster), openbaov1alpha1.PhaseDemotingBlue)
	}
}

func TestBlueGreenManager_DemotingBlue_LeaderLabelLag_UsesHealthFallback(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "bluegreen-leader-fallback")
	cluster.Spec.Replicas = 1
	cluster.Spec.UpdateStrategy = openbaov1alpha1.UpdateStrategy{
		Type: openbaov1alpha1.UpdateStrategyBlueGreen,
	}
	cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{
		ExecutorImage: "openbao/upgrade-executor:dev",
		JWTAuthRole:   "upgrade",
	}
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}
	createTLSSecret(t, namespace, cluster.Name)

	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
		status.CurrentVersion = "2.4.3"
		status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
			Phase:         openbaov1alpha1.PhaseDemotingBlue,
			BlueRevision:  "blue123",
			GreenRevision: "green456",
		}
	})

	greenPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-green456-0",
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppInstance:     cluster.Name,
				constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
				constants.LabelOpenBaoRevision: "green456",
				openbaoapi.LabelSealed:         "false",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "noop",
					Image:   "busybox:1.36",
					Command: []string{"sh", "-c", "true"},
				},
			},
		},
	}
	if err := k8sClient.Create(ctx, greenPod); err != nil {
		t.Fatalf("create pod: %v", err)
	}
	greenPod.Status.Phase = corev1.PodRunning
	greenPod.Status.Conditions = []corev1.PodCondition{
		{
			Type:   corev1.PodReady,
			Status: corev1.ConditionTrue,
		},
	}
	if err := k8sClient.Status().Update(ctx, greenPod); err != nil {
		t.Fatalf("update pod status: %v", err)
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: greenPod.Name}, greenPod); err != nil {
		t.Fatalf("get pod: %v", err)
	}
	ready := false
	for i := range greenPod.Status.Conditions {
		cond := &greenPod.Status.Conditions[i]
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatalf("expected green pod to be Ready in test setup")
	}

	infraMgr := infra.NewManager(k8sClient, k8sScheme, "openbao-operator-system", "", nil, "")
	mgr := bluegreen.NewManagerWithClientFactory(k8sClient, k8sScheme, infraMgr, func(config openbaoapi.ClientConfig) (openbaoapi.ClusterActions, error) {
		return &openbaoapi.MockClusterActions{
			IsLeaderFunc: func(ctx context.Context) (bool, error) {
				return true, nil
			},
		}, nil
	}, openbaoapi.ClientConfig{},
		security.NewImageVerifier(logr.Discard(), k8sClient, nil),
		security.NewImageVerifier(logr.Discard(), k8sClient, nil),
		"")

	latestCluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, latestCluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}

	result, err := mgr.Reconcile(ctx, logr.Discard(), latestCluster)
	if err != nil {
		t.Fatalf("reconcile DemotingBlue: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected requeue")
	}
	demoteJob := findUpgradeJobByAction(t, namespace, string(bluegreen.ActionDemoteBlueNonVotersStepDown))
	markJobSucceeded(t, demoteJob)
	demoteJob = findUpgradeJobByAction(t, namespace, string(bluegreen.ActionDemoteBlueNonVotersStepDown))
	if demoteJob.Status.Succeeded == 0 {
		t.Fatalf("expected demotion job to be marked succeeded")
	}

	// Second reconcile should not stall due to missing leader labels; the API fallback should allow progress.
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, latestCluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	result, err = mgr.Reconcile(ctx, logr.Discard(), latestCluster)
	if err != nil {
		t.Fatalf("reconcile DemotingBlue after job success: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected requeue on phase transition")
	}
	if latestCluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseCleanup {
		t.Fatalf("phase=%s want=%s", latestCluster.Status.BlueGreen.Phase, openbaov1alpha1.PhaseCleanup)
	}
}

func findUpgradeJobByAction(t *testing.T, namespace, action string) *batchv1.Job {
	t.Helper()

	var jobs batchv1.JobList
	if err := k8sClient.List(ctx, &jobs, client.InNamespace(namespace)); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	for i := range jobs.Items {
		j := &jobs.Items[i]
		if j.Annotations != nil && j.Annotations["openbao.org/upgrade-action"] == action {
			return j
		}
	}
	t.Fatalf("expected job with annotation openbao.org/upgrade-action=%q", action)
	return nil
}

func markJobSucceeded(t *testing.T, job *batchv1.Job) {
	t.Helper()
	job.Status.Succeeded = 1
	if err := k8sClient.Status().Update(ctx, job); err != nil {
		t.Fatalf("update job status: %v", err)
	}
}

func phaseOrEmpty(cluster *openbaov1alpha1.OpenBaoCluster) openbaov1alpha1.BlueGreenPhase {
	if cluster.Status.BlueGreen == nil {
		return ""
	}
	return cluster.Status.BlueGreen.Phase
}
