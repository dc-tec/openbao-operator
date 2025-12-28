package bluegreen

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
	"github.com/openbao/operator/internal/infra"
	openbaoapi "github.com/openbao/operator/internal/openbao"
)

func TestManager_Reconcile_SkipsWhenNotBlueGreen(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:        "2.4.4",
			UpdateStrategy: openbaov1alpha1.UpdateStrategy{Type: openbaov1alpha1.UpdateStrategyRollingUpdate},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	infraMgr := infra.NewManager(c, scheme, "openbao-operator-system", "", nil)
	mgr := NewManager(c, scheme, infraMgr)

	requeue, err := mgr.Reconcile(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if requeue {
		t.Fatalf("expected no requeue")
	}
}

//nolint:gocyclo // Multi-phase test requires sequential reconcile calls
func TestManager_Reconcile_CreatesJobsAndAdvancesPhases(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bluegreen",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			UpdateStrategy: openbaov1alpha1.UpdateStrategy{
				Type: openbaov1alpha1.UpdateStrategyBlueGreen,
			},
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				ExecutorImage: "openbao/upgrade-executor:dev",
				JWTAuthRole:   "upgrade",
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:         openbaov1alpha1.PhaseJoiningMesh,
				BlueRevision:  "blue123",
				GreenRevision: "green456",
				StartTime:     &metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
			},
		},
	}

	greenLeaderPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bluegreen-green456-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelAppInstance:     "bluegreen",
				constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
				constants.LabelOpenBaoRevision: "green456",
				openbaoapi.LabelActive:         "true",
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, greenLeaderPod).Build()
	infraMgr := infra.NewManager(c, scheme, "openbao-operator-system", "", nil)
	mgr := NewManager(c, scheme, infraMgr)

	ctx := context.Background()

	// Phase: JoiningMesh -> create join job
	requeue, err := mgr.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile JoiningMesh: %v", err)
	}
	if !requeue {
		t.Fatalf("expected requeue")
	}

	joinJobName := executorJobName(cluster.Name, ActionJoinGreenNonVoters, "", "blue123", "green456")
	joinJob := &batchv1.Job{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: joinJobName}, joinJob); err != nil {
		t.Fatalf("expected join job to exist: %v", err)
	}

	joinJob.Status.Succeeded = 1
	if err := c.Status().Update(ctx, joinJob); err != nil {
		t.Fatalf("mark join job succeeded: %v", err)
	}

	requeue, err = mgr.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile after join job success: %v", err)
	}
	if !requeue {
		t.Fatalf("expected requeue")
	}
	if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseSyncing {
		t.Fatalf("phase=%s want=%s", cluster.Status.BlueGreen.Phase, openbaov1alpha1.PhaseSyncing)
	}

	// Phase: Syncing -> create wait sync job
	requeue, err = mgr.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile Syncing: %v", err)
	}
	if !requeue {
		t.Fatalf("expected requeue")
	}

	// Phase: Syncing -> create wait sync job
	syncJobName := executorJobName(cluster.Name, ActionWaitGreenSynced, "", "blue123", "green456")
	syncJob := &batchv1.Job{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: syncJobName}, syncJob); err != nil {
		t.Fatalf("expected sync job to exist: %v", err)
	}

	syncJob.Status.Succeeded = 1
	if err := c.Status().Update(ctx, syncJob); err != nil {
		t.Fatalf("mark sync job succeeded: %v", err)
	}

	requeue, err = mgr.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile after sync job success: %v", err)
	}
	if !requeue {
		t.Fatalf("expected requeue")
	}
	if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhasePromoting {
		t.Fatalf("phase=%s want=%s", cluster.Status.BlueGreen.Phase, openbaov1alpha1.PhasePromoting)
	}

	// Phase: Promoting -> create promote job
	requeue, err = mgr.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile Promoting: %v", err)
	}
	if !requeue {
		t.Fatalf("expected requeue")
	}

	promoteJobName := executorJobName(cluster.Name, ActionPromoteGreenVoters, "", "blue123", "green456")
	promoteJob := &batchv1.Job{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: promoteJobName}, promoteJob); err != nil {
		t.Fatalf("expected promote job to exist: %v", err)
	}

	promoteJob.Status.Succeeded = 1
	if err := c.Status().Update(ctx, promoteJob); err != nil {
		t.Fatalf("mark promote job succeeded: %v", err)
	}

	requeue, err = mgr.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile after promote job success: %v", err)
	}
	if !requeue {
		t.Fatalf("expected requeue")
	}
	if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseDemotingBlue {
		t.Fatalf("phase=%s want=%s", cluster.Status.BlueGreen.Phase, openbaov1alpha1.PhaseDemotingBlue)
	}

	// Phase: DemotingBlue -> create demote job
	requeue, err = mgr.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile DemotingBlue: %v", err)
	}
	if !requeue {
		t.Fatalf("expected requeue")
	}

	demoteJobName := executorJobName(cluster.Name, ActionDemoteBlueNonVotersStepDown, "", "blue123", "green456")
	demoteJob := &batchv1.Job{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: demoteJobName}, demoteJob); err != nil {
		t.Fatalf("expected demote job to exist: %v", err)
	}

	demoteJob.Status.Succeeded = 1
	if err := c.Status().Update(ctx, demoteJob); err != nil {
		t.Fatalf("mark demote job succeeded: %v", err)
	}

	requeue, err = mgr.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile after demote job success: %v", err)
	}
	if !requeue {
		t.Fatalf("expected requeue")
	}
	// After demotion, DemotingBlue now also verifies Green leader (former Cutover logic).
	// Since greenLeaderPod exists with openbao-active=true, we transition directly to Cleanup.
	if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseCleanup {
		t.Fatalf("phase=%s want=%s", cluster.Status.BlueGreen.Phase, openbaov1alpha1.PhaseCleanup)
	}

	// Phase: Cleanup -> remove peers job
	requeue, err = mgr.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile Cleanup (create job): %v", err)
	}
	if !requeue {
		t.Fatalf("expected requeue")
	}

	removePeersJobName := executorJobName(cluster.Name, ActionRemoveBluePeers, "", "blue123", "green456")
	removePeersJob := &batchv1.Job{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: removePeersJobName}, removePeersJob); err != nil {
		t.Fatalf("expected remove peers job to exist: %v", err)
	}
	removePeersJob.Status.Succeeded = 1
	if err := c.Status().Update(ctx, removePeersJob); err != nil {
		t.Fatalf("mark remove peers job succeeded: %v", err)
	}

	requeue, err = mgr.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile Cleanup: %v", err)
	}
	if requeue {
		t.Fatalf("expected no requeue after completion")
	}
	if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
		t.Fatalf("phase=%s want=%s", cluster.Status.BlueGreen.Phase, openbaov1alpha1.PhaseIdle)
	}
	if cluster.Status.CurrentVersion != cluster.Spec.Version {
		t.Fatalf("currentVersion=%s want=%s", cluster.Status.CurrentVersion, cluster.Spec.Version)
	}
}
