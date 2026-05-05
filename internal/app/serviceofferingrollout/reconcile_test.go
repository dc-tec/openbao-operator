package serviceofferingrollout

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const (
	testRolloutName = "standard-v2-rollout"
	testOffering    = "standard"
	testProfileV1   = "standard-v1"
	testProfileV2   = "standard-v2"
)

func TestReconcile_CreatesUpgradeRequestForEligibleClaim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rollout := newRollout()
	reconciler := newTestReconciler(t,
		rollout,
		newOffering(testProfileV2),
		newProfile(),
		newClaim("bao-a", testProfileV1),
	)

	result, err := reconciler.Reconcile(ctx, client.ObjectKeyFromObject(rollout), logr.Discard())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter != constants.RequeueShort {
		t.Fatalf("Reconcile() requeueAfter = %s, want %s", result.RequeueAfter, constants.RequeueShort)
	}

	requests := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList{}
	if err := reconciler.client.List(ctx, requests); err != nil {
		t.Fatalf("list upgrade requests: %v", err)
	}
	if len(requests.Items) != 1 {
		t.Fatalf("upgrade request count = %d, want 1", len(requests.Items))
	}
	request := requests.Items[0]
	if request.Namespace != "payments" || request.Spec.ClaimRef.Name != "bao-a" {
		t.Fatalf("request target = %s/%s, want payments/bao-a", request.Namespace, request.Spec.ClaimRef.Name)
	}
	if request.Spec.Target.ServiceOfferingRef == nil || request.Spec.Target.ServiceOfferingRef.Name != testOffering {
		t.Fatalf("request serviceOfferingRef = %#v, want %s", request.Spec.Target.ServiceOfferingRef, testOffering)
	}
	if request.Labels[constants.LabelOpenBaoServiceOfferingRollout] != rollout.Name {
		t.Fatalf("request rollout label = %q, want %q", request.Labels[constants.LabelOpenBaoServiceOfferingRollout], rollout.Name)
	}
	if request.Annotations[constants.AnnotationServiceOfferingRolloutUID] != string(rollout.UID) {
		t.Fatalf("request rollout uid annotation = %q, want %q", request.Annotations[constants.AnnotationServiceOfferingRolloutUID], rollout.UID)
	}

	updated := &openbaov1alpha1.OpenBaoServiceOfferingRollout{}
	if err := reconciler.client.Get(ctx, client.ObjectKeyFromObject(rollout), updated); err != nil {
		t.Fatalf("get rollout: %v", err)
	}
	if updated.Status.State != openbaov1alpha1.OpenBaoServiceOfferingRolloutStateRunning {
		t.Fatalf("rollout state = %q, want Running", updated.Status.State)
	}
	if updated.Status.Total != 1 || updated.Status.Pending != 1 {
		t.Fatalf("rollout counts total/pending = %d/%d, want 1/1", updated.Status.Total, updated.Status.Pending)
	}
	if updated.Status.TargetRevisionRef == nil || updated.Status.TargetRevisionRef.Name != testProfileV2 {
		t.Fatalf("target revision status = %#v, want %s", updated.Status.TargetRevisionRef, testProfileV2)
	}
}

func TestReconcile_RespectsMaxConcurrent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	maxConcurrent := int32(1)
	rollout := newRollout()
	rollout.Spec.Strategy = &openbaov1alpha1.OpenBaoServiceOfferingRolloutStrategySpec{
		MaxConcurrent: &maxConcurrent,
	}
	existing := newRolloutUpgradeRequest(rollout, "payments", "bao-a", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut)
	reconciler := newTestReconciler(t,
		rollout,
		newOffering(testProfileV2),
		newProfile(),
		newClaim("bao-a", testProfileV1),
		newClaim("bao-b", testProfileV1),
		existing,
	)

	if _, err := reconciler.Reconcile(ctx, client.ObjectKeyFromObject(rollout), logr.Discard()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	requests := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList{}
	if err := reconciler.client.List(ctx, requests); err != nil {
		t.Fatalf("list upgrade requests: %v", err)
	}
	if len(requests.Items) != 1 {
		t.Fatalf("upgrade request count = %d, want 1", len(requests.Items))
	}
	updated := &openbaov1alpha1.OpenBaoServiceOfferingRollout{}
	if err := reconciler.client.Get(ctx, client.ObjectKeyFromObject(rollout), updated); err != nil {
		t.Fatalf("get rollout: %v", err)
	}
	if updated.Status.Total != 2 || updated.Status.Pending != 1 || updated.Status.Running != 1 {
		t.Fatalf("rollout counts total/pending/running = %d/%d/%d, want 2/1/1", updated.Status.Total, updated.Status.Pending, updated.Status.Running)
	}
	if len(updated.Status.Claims) != 2 || updated.Status.Claims[1].Reason != reasonWaitingForRolloutSlot {
		t.Fatalf("claim statuses = %#v, want second claim waiting for rollout slot", updated.Status.Claims)
	}
}

func TestReconcile_WaitsForExistingActiveUpgradeRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rollout := newRollout()
	existing := newUpgradeRequest("manual-upgrade", "payments", "bao-a", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut)
	reconciler := newTestReconciler(t,
		rollout,
		newOffering(testProfileV2),
		newProfile(),
		newClaim("bao-a", testProfileV1),
		existing,
	)

	if _, err := reconciler.Reconcile(ctx, client.ObjectKeyFromObject(rollout), logr.Discard()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	requests := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList{}
	if err := reconciler.client.List(ctx, requests); err != nil {
		t.Fatalf("list upgrade requests: %v", err)
	}
	if len(requests.Items) != 1 {
		t.Fatalf("upgrade request count = %d, want only the existing request", len(requests.Items))
	}
	updated := &openbaov1alpha1.OpenBaoServiceOfferingRollout{}
	if err := reconciler.client.Get(ctx, client.ObjectKeyFromObject(rollout), updated); err != nil {
		t.Fatalf("get rollout: %v", err)
	}
	if updated.Status.Pending != 1 || len(updated.Status.Claims) != 1 {
		t.Fatalf("rollout pending/status count = %d/%d, want 1/1", updated.Status.Pending, len(updated.Status.Claims))
	}
	if updated.Status.Claims[0].Reason != reasonAnotherUpgradeRequestActive {
		t.Fatalf("claim reason = %q, want %s", updated.Status.Claims[0].Reason, reasonAnotherUpgradeRequestActive)
	}
	if updated.Status.Claims[0].RequestRef == nil || updated.Status.Claims[0].RequestRef.Name != existing.Name {
		t.Fatalf("claim request ref = %#v, want %s", updated.Status.Claims[0].RequestRef, existing.Name)
	}
}

func TestReconcile_BlocksWhenTargetRevisionIsNotCurrentOfferingRevision(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rollout := newRollout()
	reconciler := newTestReconciler(t,
		rollout,
		newOffering(testProfileV1),
	)

	if _, err := reconciler.Reconcile(ctx, client.ObjectKeyFromObject(rollout), logr.Discard()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &openbaov1alpha1.OpenBaoServiceOfferingRollout{}
	if err := reconciler.client.Get(ctx, client.ObjectKeyFromObject(rollout), updated); err != nil {
		t.Fatalf("get rollout: %v", err)
	}
	if updated.Status.State != openbaov1alpha1.OpenBaoServiceOfferingRolloutStateBlocked {
		t.Fatalf("rollout state = %q, want Blocked", updated.Status.State)
	}
	if updated.Status.Reason != reasonTargetRevisionNotCurrent {
		t.Fatalf("rollout reason = %q, want %s", updated.Status.Reason, reasonTargetRevisionNotCurrent)
	}
}

func TestReconcile_TreatsAlreadyAppliedTargetAsSucceeded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rollout := newRollout()
	reconciler := newTestReconciler(t,
		rollout,
		newOffering(testProfileV2),
		newProfile(),
		newClaim("bao-a", testProfileV2),
	)

	result, err := reconciler.Reconcile(ctx, client.ObjectKeyFromObject(rollout), logr.Discard())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("Reconcile() requeueAfter = %s, want 0", result.RequeueAfter)
	}

	updated := &openbaov1alpha1.OpenBaoServiceOfferingRollout{}
	if err := reconciler.client.Get(ctx, client.ObjectKeyFromObject(rollout), updated); err != nil {
		t.Fatalf("get rollout: %v", err)
	}
	if updated.Status.State != openbaov1alpha1.OpenBaoServiceOfferingRolloutStateSucceeded {
		t.Fatalf("rollout state = %q, want Succeeded", updated.Status.State)
	}
	if updated.Status.Total != 1 || updated.Status.Succeeded != 1 {
		t.Fatalf("rollout counts total/succeeded = %d/%d, want 1/1", updated.Status.Total, updated.Status.Succeeded)
	}
	if len(updated.Status.Claims) != 1 || updated.Status.Claims[0].Reason != reasonAlreadyApplied {
		t.Fatalf("claim statuses = %#v, want already applied reason", updated.Status.Claims)
	}
}

func TestReconcile_ReflectsBlockedRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rollout := newRollout()
	existing := newRolloutUpgradeRequest(rollout, "payments", "bao-a", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked)
	existing.Status.Reason = "UnsupportedServiceShapeChange"
	reconciler := newTestReconciler(t,
		rollout,
		newOffering(testProfileV2),
		newProfile(),
		newClaim("bao-a", testProfileV1),
		existing,
	)

	if _, err := reconciler.Reconcile(ctx, client.ObjectKeyFromObject(rollout), logr.Discard()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &openbaov1alpha1.OpenBaoServiceOfferingRollout{}
	if err := reconciler.client.Get(ctx, client.ObjectKeyFromObject(rollout), updated); err != nil {
		t.Fatalf("get rollout: %v", err)
	}
	if updated.Status.State != openbaov1alpha1.OpenBaoServiceOfferingRolloutStateBlocked {
		t.Fatalf("rollout state = %q, want Blocked", updated.Status.State)
	}
	if updated.Status.Blocked != 1 {
		t.Fatalf("blocked count = %d, want 1", updated.Status.Blocked)
	}
	if len(updated.Status.Claims) != 1 || updated.Status.Claims[0].Reason != "UnsupportedServiceShapeChange" {
		t.Fatalf("claim statuses = %#v, want blocked request reason", updated.Status.Claims)
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	return scheme
}

func newTestReconciler(t *testing.T, objects ...client.Object) runtimeReconciler {
	t.Helper()

	scheme := newTestScheme(t)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&openbaov1alpha1.OpenBaoServiceOfferingRollout{},
			&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{},
		).
		WithObjects(objects...).
		Build()
	return runtimeReconciler{
		client:              fakeClient,
		reader:              fakeClient,
		enableServiceClaims: true,
	}
}

func newRollout() *openbaov1alpha1.OpenBaoServiceOfferingRollout {
	return &openbaov1alpha1.OpenBaoServiceOfferingRollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testRolloutName,
			UID:        types.UID("rollout-uid"),
			Generation: 1,
		},
		Spec: openbaov1alpha1.OpenBaoServiceOfferingRolloutSpec{
			OfferingRef:       openbaov1alpha1.LocalReference{Name: testOffering},
			TargetRevisionRef: openbaov1alpha1.LocalReference{Name: testProfileV2},
		},
	}
}

func newOffering(revision string) *openbaov1alpha1.OpenBaoServiceOffering {
	return &openbaov1alpha1.OpenBaoServiceOffering{
		ObjectMeta: metav1.ObjectMeta{Name: testOffering, UID: types.UID(testOffering + "-uid")},
		Spec: openbaov1alpha1.OpenBaoServiceOfferingSpec{
			CurrentRevisionRef: openbaov1alpha1.LocalReference{Name: revision},
		},
	}
}

func newProfile() *openbaov1alpha1.OpenBaoServiceProfile {
	return &openbaov1alpha1.OpenBaoServiceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: testProfileV2, UID: types.UID(testProfileV2 + "-uid")},
	}
}

func newClaim(name, profile string) *openbaov1alpha1.OpenBaoClusterClaim {
	return &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      name,
			UID:       types.UID("payments-" + name + "-uid"),
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:          openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: testOffering},
			ServiceProfileRef:  openbaov1alpha1.LocalReference{Name: profile},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimStatus{
			Applied: openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
				ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: testOffering},
				ServiceProfileRef: &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
					Name: profile,
					UID:  profile + "-uid",
				},
			},
		},
	}
}

func newRolloutUpgradeRequest(
	rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout,
	namespace string,
	claimName string,
	state openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest {
	request := newUpgradeRequest(rolloutRequestName(rollout.Name, namespace, claimName), namespace, claimName, state)
	request.Labels = map[string]string{
		constants.LabelOpenBaoServiceOfferingRollout: rollout.Name,
		constants.LabelOpenBaoClaimNamespace:         namespace,
		constants.LabelOpenBaoClaimName:              claimName,
	}
	request.Annotations = map[string]string{
		constants.AnnotationServiceOfferingRolloutUID: string(rollout.UID),
	}
	return request
}

func newUpgradeRequest(
	name string,
	namespace string,
	claimName string,
	state openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest {
	return &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: claimName},
			Target: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
				ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: testOffering},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatus{
			State: state,
		},
	}
}
