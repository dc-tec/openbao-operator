package bluegreen

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/infra"
	"github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/security"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

func TestRunExecutorJob_FailedJob_RetriesWithRunIDWhenEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = openbaov1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	maxFailures := int32(2)
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.4.4",
			UpdateStrategy: openbaov1alpha1.UpdateStrategy{
				Type: openbaov1alpha1.UpdateStrategyBlueGreen,
				BlueGreen: &openbaov1alpha1.BlueGreenConfig{
					MaxJobFailures: &maxFailures,
				},
			},
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				ExecutorImage: "example.com/upgrade:latest",
				JWTAuthRole:   "upgrade",
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:         openbaov1alpha1.PhaseJoiningMesh,
				BlueRevision:  "blue",
				GreenRevision: "green",
			},
		},
	}

	jobName := upgrade.ExecutorJobName(cluster.Name, ActionJoinGreenNonVoters, "", "blue", "green")
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{Failed: 1},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, job).
		Build()

	infraMgr := infra.NewManager(c, scheme, "openbao-operator-system", "", nil, "")
	mgr := NewManager(c, scheme, infraMgr, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), c, nil), security.NewImageVerifier(logr.Discard(), c, nil), "")

	step, err := mgr.runExecutorJobStep(context.Background(), logr.Discard(), cluster, ActionJoinGreenNonVoters, "job failure threshold exceeded")
	if err != nil {
		t.Fatalf("runExecutorJobStep: %v", err)
	}
	if step.Completed {
		t.Fatalf("expected executor job step to be incomplete")
	}
	if step.Outcome.kind != phaseOutcomeRequeueAfter {
		t.Fatalf("expected outcome %s, got %s", phaseOutcomeRequeueAfter, step.Outcome.kind)
	}
	if cluster.Status.BlueGreen.JobFailureCount != 1 {
		t.Fatalf("expected JobFailureCount=1, got %d", cluster.Status.BlueGreen.JobFailureCount)
	}
	if cluster.Status.BlueGreen.LastJobFailure != jobName {
		t.Fatalf("expected LastJobFailure=%q, got %q", jobName, cluster.Status.BlueGreen.LastJobFailure)
	}

	// Next reconcile should create a new job attempt with runID retry-1.
	step, err = mgr.runExecutorJobStep(context.Background(), logr.Discard(), cluster, ActionJoinGreenNonVoters, "job failure threshold exceeded")
	if err != nil {
		t.Fatalf("runExecutorJobStep (retry attempt): %v", err)
	}
	if step.Completed {
		t.Fatalf("expected executor job step to be incomplete while retry job is running/created")
	}
	if step.Outcome.kind != phaseOutcomeRequeueAfter {
		t.Fatalf("expected outcome %s, got %s", phaseOutcomeRequeueAfter, step.Outcome.kind)
	}
	retryJobName := upgrade.ExecutorJobName(cluster.Name, ActionJoinGreenNonVoters, "retry-1", "blue", "green")
	retryJob := &batchv1.Job{}
	if getErr := c.Get(context.Background(), types.NamespacedName{Namespace: cluster.Namespace, Name: retryJobName}, retryJob); getErr != nil {
		t.Fatalf("expected retry job %s/%s to be created: %v", cluster.Namespace, retryJobName, getErr)
	}
}

func TestRunExecutorJob_FailedJob_DoesNotRetryWhenAutoRollbackDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = openbaov1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.4.4",
			UpdateStrategy: openbaov1alpha1.UpdateStrategy{
				Type: openbaov1alpha1.UpdateStrategyBlueGreen,
				BlueGreen: &openbaov1alpha1.BlueGreenConfig{
					AutoRollback: &openbaov1alpha1.AutoRollbackConfig{
						Enabled:      false,
						OnJobFailure: true,
					},
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:         openbaov1alpha1.PhaseJoiningMesh,
				BlueRevision:  "blue",
				GreenRevision: "green",
			},
		},
	}

	jobName := upgrade.ExecutorJobName(cluster.Name, ActionJoinGreenNonVoters, "", "blue", "green")
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{Failed: 1},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, job).
		Build()

	infraMgr := infra.NewManager(c, scheme, "openbao-operator-system", "", nil, "")
	mgr := NewManager(c, scheme, infraMgr, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), c, nil), security.NewImageVerifier(logr.Discard(), c, nil), "")

	step, err := mgr.runExecutorJobStep(context.Background(), logr.Discard(), cluster, ActionJoinGreenNonVoters, "job failure threshold exceeded")
	if err != nil {
		t.Fatalf("runExecutorJobStep: %v", err)
	}
	if step.Completed {
		t.Fatalf("expected executor job step to be incomplete when auto rollback/retry is disabled")
	}
	if step.Outcome.kind != phaseOutcomeHold {
		t.Fatalf("expected outcome %s, got %s", phaseOutcomeHold, step.Outcome.kind)
	}
	if cluster.Status.BlueGreen.JobFailureCount != 1 {
		t.Fatalf("expected JobFailureCount=1, got %d", cluster.Status.BlueGreen.JobFailureCount)
	}

	// Re-observing the same failed job should not cause new retry jobs to be created.
	step, err = mgr.runExecutorJobStep(context.Background(), logr.Discard(), cluster, ActionJoinGreenNonVoters, "job failure threshold exceeded")
	if err != nil {
		t.Fatalf("runExecutorJobStep (repeat): %v", err)
	}
	if step.Completed {
		t.Fatalf("expected executor job step to remain incomplete when auto rollback/retry is disabled")
	}
	if step.Outcome.kind != phaseOutcomeHold {
		t.Fatalf("expected outcome %s, got %s", phaseOutcomeHold, step.Outcome.kind)
	}
	if cluster.Status.BlueGreen.JobFailureCount != 1 {
		t.Fatalf("expected JobFailureCount to remain 1, got %d", cluster.Status.BlueGreen.JobFailureCount)
	}
}

func TestRunExecutorJob_FailedJob_TriggersAbortWhenMaxFailuresReached(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = openbaov1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	maxFailures := int32(1)
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.4.4",
			UpdateStrategy: openbaov1alpha1.UpdateStrategy{
				Type: openbaov1alpha1.UpdateStrategyBlueGreen,
				BlueGreen: &openbaov1alpha1.BlueGreenConfig{
					MaxJobFailures: &maxFailures,
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:         openbaov1alpha1.PhaseJoiningMesh,
				BlueRevision:  "blue",
				GreenRevision: "green",
			},
		},
	}

	jobName := upgrade.ExecutorJobName(cluster.Name, ActionJoinGreenNonVoters, "", "blue", "green")
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{Failed: 1},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, job).
		Build()

	infraMgr := infra.NewManager(c, scheme, "openbao-operator-system", "", nil, "")
	mgr := NewManager(c, scheme, infraMgr, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), c, nil), security.NewImageVerifier(logr.Discard(), c, nil), "")

	step, err := mgr.runExecutorJobStep(context.Background(), logr.Discard(), cluster, ActionJoinGreenNonVoters, "job failure threshold exceeded")
	if err != nil {
		t.Fatalf("runExecutorJobStep: %v", err)
	}
	if step.Completed {
		t.Fatalf("expected executor job step to be incomplete on failure threshold exceeded")
	}
	if step.Outcome.kind != phaseOutcomeRollback {
		t.Fatalf("expected outcome %s, got %s", phaseOutcomeRollback, step.Outcome.kind)
	}

	_, err = mgr.applyOutcome(context.Background(), logr.Discard(), cluster, step.Outcome)
	if err != nil {
		t.Fatalf("applyOutcome: %v", err)
	}
	if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
		t.Fatalf("expected phase Idle after abort, got %s", cluster.Status.BlueGreen.Phase)
	}
	if cluster.Status.BlueGreen.GreenRevision != "" {
		t.Fatalf("expected greenRevision to be cleared after abort, got %q", cluster.Status.BlueGreen.GreenRevision)
	}
	if cluster.Status.BlueGreen.JobFailureCount != 0 {
		t.Fatalf("expected JobFailureCount to be reset after abort, got %d", cluster.Status.BlueGreen.JobFailureCount)
	}
}
