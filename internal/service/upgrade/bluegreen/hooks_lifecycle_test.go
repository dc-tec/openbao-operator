package bluegreen

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminopsstatus"
)

func testBlueGreenAdminOpsMutator(c client.Client) adminOpsStatusMutator {
	return func(
		ctx context.Context,
		cluster *openbaov1alpha1.OpenBaoCluster,
		mutate func(obj *openbaov1alpha1.OpenBaoCluster) error,
		forceOwnership bool,
	) error {
		return adminopsstatus.MutateWithReader(ctx, c, c, cluster, mutate, adminopsstatus.MutateOptions{
			ForceOwnership:  forceOwnership,
			RetryOnConflict: !forceOwnership,
		})
	}
}

func newValidationHookLifecycleTest(t *testing.T) (*openbaov1alpha1.OpenBaoCluster, *openbaov1alpha1.ValidationHookConfig, client.Client, *Manager) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	hook := &openbaov1alpha1.ValidationHookConfig{
		Image:   "registry.example.com/validation-hook:v1",
		Command: []string{"/bin/validate"},
		Args:    []string{"--green"},
	}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
			UID:       types.UID("cluster-uid"),
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				BlueGreen: &openbaov1alpha1.BlueGreenConfig{
					Verification: &openbaov1alpha1.VerificationConfig{PrePromotionHook: hook},
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:         openbaov1alpha1.PhaseSyncing,
				OperationID:   "operation-1",
				GreenRevision: "green-revision",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}, &batchv1.Job{}).
		WithObjects(cluster.DeepCopy()).
		Build()
	manager := (&Manager{client: k8sClient, reader: k8sClient, scheme: scheme}).
		WithAdminOpsStatusMutator(testBlueGreenAdminOpsMutator(k8sClient))
	return cluster, hook, k8sClient, manager
}

func prepareAndCreateValidationHook(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster, hook *openbaov1alpha1.ValidationHookConfig, manager *Manager) *batchv1.Job {
	t.Helper()

	result, receiptAdvanced, err := manager.reconcilePrePromotionHookJob(context.Background(), logr.Discard(), cluster, hook)
	if err != nil {
		t.Fatalf("prepare hook: %v", err)
	}
	if result != nil || !receiptAdvanced {
		t.Fatalf("prepare hook result = %+v, receiptAdvanced = %v", result, receiptAdvanced)
	}
	if cluster.Status.BlueGreen.ValidationHook == nil || cluster.Status.BlueGreen.ValidationHook.Stage != openbaov1alpha1.BlueGreenValidationHookStagePrepared {
		t.Fatalf("prepared receipt = %+v", cluster.Status.BlueGreen.ValidationHook)
	}

	result, receiptAdvanced, err = manager.reconcilePrePromotionHookJob(context.Background(), logr.Discard(), cluster, hook)
	if err != nil {
		t.Fatalf("create hook: %v", err)
	}
	if result == nil || !result.Running || !receiptAdvanced {
		t.Fatalf("create hook result = %+v, receiptAdvanced = %v", result, receiptAdvanced)
	}
	if cluster.Status.BlueGreen.ValidationHook.Stage != openbaov1alpha1.BlueGreenValidationHookStageCreated {
		t.Fatalf("created receipt = %+v", cluster.Status.BlueGreen.ValidationHook)
	}

	job := &batchv1.Job{}
	if err := manager.client.Get(context.Background(), client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Status.BlueGreen.ValidationHook.JobName,
	}, job); err != nil {
		t.Fatalf("get validation hook Job: %v", err)
	}
	return job
}

func TestValidationHookIdentityIncludesAttemptRevisionAndNormalizedSpec(t *testing.T) {
	t.Parallel()

	hookWithDefaults := &openbaov1alpha1.ValidationHookConfig{Image: "example.com/hook:v1"}
	timeout := defaultValidationHookTimeoutSeconds
	hookWithExplicitDefaults := &openbaov1alpha1.ValidationHookConfig{
		Image:          "example.com/hook:v1",
		Command:        []string{},
		Args:           []string{},
		TimeoutSeconds: &timeout,
	}
	firstHash, err := validationHookSpecHash(hookWithDefaults)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := validationHookSpecHash(hookWithExplicitDefaults)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("normalized hashes differ: %q != %q", firstHash, secondHash)
	}

	base := validationHookJobName("cluster", "operation-1", "green-1", firstHash)
	if base != validationHookJobName("cluster", "operation-1", "green-1", firstHash) {
		t.Fatal("validation hook name is not stable")
	}
	if base == validationHookJobName("cluster", "operation-2", "green-1", firstHash) {
		t.Fatal("validation hook name does not include the operation ID")
	}
	if base == validationHookJobName("cluster", "operation-1", "green-2", firstHash) {
		t.Fatal("validation hook name does not include the Green revision")
	}
	changedHash, err := validationHookSpecHash(&openbaov1alpha1.ValidationHookConfig{Image: "example.com/hook:v2"})
	if err != nil {
		t.Fatal(err)
	}
	if base == validationHookJobName("cluster", "operation-1", "green-1", changedHash) {
		t.Fatal("validation hook name does not include the normalized hook specification")
	}
}

func TestValidationHookMissingAfterCreationBecomesUnknownWithoutReplay(t *testing.T) {
	t.Parallel()

	cluster, hook, k8sClient, manager := newValidationHookLifecycleTest(t)
	job := prepareAndCreateValidationHook(t, cluster, hook, manager)
	if err := k8sClient.Delete(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	_, _, err := manager.reconcilePrePromotionHookJob(context.Background(), logr.Discard(), cluster, hook)
	if err == nil || !strings.Contains(err.Error(), "is Unknown") {
		t.Fatalf("missing Job error = %v, want Unknown", err)
	}
	if cluster.Status.BlueGreen.ValidationHook.Stage != openbaov1alpha1.BlueGreenValidationHookStageUnknown {
		t.Fatalf("stage = %q, want Unknown", cluster.Status.BlueGreen.ValidationHook.Stage)
	}

	_, _, err = manager.reconcilePrePromotionHookJob(context.Background(), logr.Discard(), cluster, hook)
	if err == nil || !strings.Contains(err.Error(), "will not be recreated") {
		t.Fatalf("Unknown receipt error = %v", err)
	}
	jobs := &batchv1.JobList{}
	if err := k8sClient.List(context.Background(), jobs, client.InNamespace(cluster.Namespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("Jobs recreated after Unknown receipt: %+v", jobs.Items)
	}
}

func TestValidationHookMissingAfterCommitmentBecomesUnknown(t *testing.T) {
	t.Parallel()

	cluster, hook, k8sClient, manager := newValidationHookLifecycleTest(t)
	_, receiptAdvanced, err := manager.reconcilePrePromotionHookJob(context.Background(), logr.Discard(), cluster, hook)
	if err != nil || !receiptAdvanced {
		t.Fatalf("prepare hook: receiptAdvanced = %v, error = %v", receiptAdvanced, err)
	}
	cluster.Status.BlueGreen.ValidationHook.Stage = openbaov1alpha1.BlueGreenValidationHookStageCommitted
	now := metav1.Now()
	cluster.Status.BlueGreen.ValidationHook.CommittedAt = &now
	if err := manager.persistBlueGreenStatus(context.Background(), cluster); err != nil {
		t.Fatal(err)
	}

	_, _, err = manager.reconcilePrePromotionHookJob(context.Background(), logr.Discard(), cluster, hook)
	if err == nil || !strings.Contains(err.Error(), "is Unknown") {
		t.Fatalf("committed missing Job error = %v, want Unknown", err)
	}
	if cluster.Status.BlueGreen.ValidationHook.Stage != openbaov1alpha1.BlueGreenValidationHookStageUnknown {
		t.Fatalf("stage = %q, want Unknown", cluster.Status.BlueGreen.ValidationHook.Stage)
	}
	jobs := &batchv1.JobList{}
	if err := k8sClient.List(context.Background(), jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("unexpected Jobs after committed receipt recovery: %+v", jobs.Items)
	}
}

func TestValidationHookRetainedUntilTerminalReceiptAndPhaseAdvance(t *testing.T) {
	t.Parallel()

	cluster, hook, k8sClient, manager := newValidationHookLifecycleTest(t)
	job := prepareAndCreateValidationHook(t, cluster, hook, manager)
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	job.Status.Succeeded = 1
	if err := k8sClient.Status().Update(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	result, receiptAdvanced, err := manager.reconcilePrePromotionHookJob(context.Background(), logr.Discard(), cluster, hook)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.Succeeded || !receiptAdvanced {
		t.Fatalf("terminal observation result = %+v, receiptAdvanced = %v", result, receiptAdvanced)
	}
	if cluster.Status.BlueGreen.ValidationHook.Stage != openbaov1alpha1.BlueGreenValidationHookStageTerminalObserved {
		t.Fatalf("terminal receipt = %+v", cluster.Status.BlueGreen.ValidationHook)
	}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(job), &batchv1.Job{}); err != nil {
		t.Fatalf("terminal Job was removed before phase advancement: %v", err)
	}

	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhasePromoting
	if err := manager.persistBlueGreenStatus(context.Background(), cluster); err != nil {
		t.Fatal(err)
	}
	handled, resultAfter, err := manager.reconcileValidationHookOutsideSyncing(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if !handled || resultAfter.RequeueAfter <= 0 {
		t.Fatalf("cleanup handled = %v, result = %+v", handled, resultAfter)
	}
	if cluster.Status.BlueGreen.ValidationHook != nil {
		t.Fatalf("validation hook receipt not cleared: %+v", cluster.Status.BlueGreen.ValidationHook)
	}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(job), &batchv1.Job{}); !apierrors.IsNotFound(err) {
		t.Fatalf("terminal Job still exists after durable phase advancement: %v", err)
	}
}
