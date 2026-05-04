package upgraderequest

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
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
)

const (
	standardOfferingName = "standard"
	standardV1Name       = "standard-v1"
	standardV2Name       = "standard-v2"
	version240           = "2.4.0"
)

func TestClassifyUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		mutateCurrent func(*claimcontract.ApprovedServiceContract, *claimcontract.CatalogBundle)
		mutateTarget  func(*claimcontract.ApprovedServiceContract, *claimcontract.CatalogBundle)
		wantClass     openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClass
		wantReason    string
	}{
		{
			name:       "equivalent service shape is in-place",
			wantClass:  openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace,
			wantReason: "EquivalentServiceShape",
		},
		{
			name: "version change is in-place",
			mutateTarget: func(contract *claimcontract.ApprovedServiceContract, _ *claimcontract.CatalogBundle) {
				contract.Cluster.Version = version240
			},
			wantClass:  openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace,
			wantReason: reasonInPlaceSupported,
		},
		{
			name: "bootstrap change is blocked",
			mutateTarget: func(contract *claimcontract.ApprovedServiceContract, _ *claimcontract.CatalogBundle) {
				contract.Bootstrap.ProfileRef = &openbaov1alpha1.LocalReference{Name: "bootstrap-v2"}
			},
			wantClass:  openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked,
			wantReason: "BootstrapChangeRequiresReprovision",
		},
		{
			name: "backup execution identity change is blocked",
			mutateTarget: func(_ *claimcontract.ApprovedServiceContract, catalog *claimcontract.CatalogBundle) {
				catalog.BackupTarget.Name = "target-v2"
			},
			wantClass:  openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked,
			wantReason: "BackupExecutionIdentityChanged",
		},
		{
			name: "exposure class change is blocked",
			mutateTarget: func(contract *claimcontract.ApprovedServiceContract, _ *claimcontract.CatalogBundle) {
				contract.Exposure.ClassRef.Name = "external-v2"
			},
			wantClass:  openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked,
			wantReason: "UnsupportedServiceShapeChange",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			currentContract, currentCatalog := baselineApprovedContractAndCatalog()
			targetContract, targetCatalog := baselineApprovedContractAndCatalog()

			if tt.mutateCurrent != nil {
				tt.mutateCurrent(currentContract, currentCatalog)
			}
			if tt.mutateTarget != nil {
				tt.mutateTarget(targetContract, targetCatalog)
			}

			gotClass, gotReason := classifyUpgrade(currentContract, currentCatalog, targetContract, targetCatalog)
			if gotClass != tt.wantClass {
				t.Fatalf("classifyUpgrade() class = %q, want %q", gotClass, tt.wantClass)
			}
			if gotReason != tt.wantReason {
				t.Fatalf("classifyUpgrade() reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

func TestReconcileRequestState_ServiceClaimsDisabled(t *testing.T) {
	t.Parallel()

	reconciler := runtimeReconciler{enableServiceClaims: false}
	state, reason, current, target, classification := reconciler.reconcileRequestState(context.Background(), &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{})
	if state != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked)
	}
	if reason != "ServiceClaimsDisabled" {
		t.Fatalf("reason = %q, want ServiceClaimsDisabled", reason)
	}
	if current != nil || target != nil {
		t.Fatalf("expected nil current/target, got %#v %#v", current, target)
	}
	if classification == nil || classification.Class != openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked {
		t.Fatalf("classification = %#v, want blocked", classification)
	}
}

func TestReconcileRequestState_PreservesTerminalStatus(t *testing.T) {
	t.Parallel()

	request := newUpgradeRequest("upgrade-terminal", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
		ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
	})
	request.Status.State = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed
	request.Status.Reason = reasonClaimRolloutFailed
	request.Status.Current = currentRevisionStatusForTest(standardV1Name, "standard-v1-uid", currentApprovedContractStatus().IdentityHash)
	request.Status.Target = currentRevisionStatusForTest(standardV2Name, "standard-v2-uid", targetApprovedContractStatus().IdentityHash)
	request.Status.Classification = classificationStatus(
		openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace,
		reasonInPlaceSupported,
	)

	reconciler := runtimeReconciler{enableServiceClaims: true}
	state, reason, current, target, classification := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed)
	}
	if reason != reasonClaimRolloutFailed {
		t.Fatalf("reason = %q, want %s", reason, reasonClaimRolloutFailed)
	}
	if current == nil || current.ServiceProfileRef == nil || current.ServiceProfileRef.Name != standardV1Name {
		t.Fatalf("current = %#v, want %s", current, standardV1Name)
	}
	if target == nil || target.ServiceProfileRef == nil || target.ServiceProfileRef.Name != standardV2Name {
		t.Fatalf("target = %#v, want %s", target, standardV2Name)
	}
	if classification == nil || classification.Class != openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace || classification.Reason != reasonInPlaceSupported {
		t.Fatalf("classification = %#v, want in-place/%s", classification, reasonInPlaceSupported)
	}
}

func TestReconcileRequestState_PromotesClaimSpecForInPlaceUpgrade(t *testing.T) {
	t.Parallel()

	reconciler := newUpgradeTestReconciler(t, upgradeCatalogObjects()...)

	request := newUpgradeRequest("upgrade-1", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
		ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
	})

	state, reason, current, target, classification := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut)
	}
	if reason != "RolloutRequested" {
		t.Fatalf("reason = %q, want RolloutRequested", reason)
	}
	if current == nil || current.ServiceProfileRef == nil || current.ServiceProfileRef.Name != standardV1Name {
		t.Fatalf("current status = %#v, want serviceProfileRef %s", current, standardV1Name)
	}
	if target == nil || target.ServiceProfileRef == nil || target.ServiceProfileRef.Name != standardV2Name {
		t.Fatalf("target status = %#v, want serviceProfileRef %s", target, standardV2Name)
	}
	if target.ServiceOfferingRef == nil || target.ServiceOfferingRef.Name != "standard" {
		t.Fatalf("target offering ref = %#v, want standard", target.ServiceOfferingRef)
	}
	if classification == nil || classification.Class != openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace || classification.Reason != reasonInPlaceSupported {
		t.Fatalf("classification = %#v, want in-place/%s", classification, reasonInPlaceSupported)
	}

	var claim openbaov1alpha1.OpenBaoClusterClaim
	if err := reconciler.client.Get(context.Background(), types.NamespacedName{Namespace: "payments", Name: "payments-bao"}, &claim); err != nil {
		t.Fatalf("get updated claim: %v", err)
	}
	if claim.Spec.ServiceProfileRef.Name != standardV2Name {
		t.Fatalf("claim spec serviceProfileRef = %q, want %s", claim.Spec.ServiceProfileRef.Name, standardV2Name)
	}
	if claim.Spec.ServiceOfferingRef == nil || claim.Spec.ServiceOfferingRef.Name != "standard" {
		t.Fatalf("claim spec serviceOfferingRef = %#v, want standard", claim.Spec.ServiceOfferingRef)
	}
	if claim.Annotations[constants.AnnotationClaimUpgradeRequest] != upgradeRequestToken(request) {
		t.Fatalf("claim upgrade request annotation = %q, want %q", claim.Annotations[constants.AnnotationClaimUpgradeRequest], upgradeRequestToken(request))
	}
}

func TestReconcileRequestState_UsesAppliedRevisionDuringRollout(t *testing.T) {
	t.Parallel()

	objects := upgradeCatalogObjects()
	for _, obj := range objects {
		claim, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaim)
		if !ok {
			continue
		}
		claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard"}
		claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: standardV2Name}
		claim.Status.Applied.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard"}
		claim.Status.Applied.ServiceProfileRef = &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
			Name: standardV1Name,
			UID:  "standard-v1-uid",
		}
		claim.Status.Applied.ApprovedContract = currentApprovedContractStatus()
	}
	reconciler := newUpgradeTestReconciler(t, objects...)

	request := newUpgradeRequest("upgrade-1", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
		ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
	})
	request.Status.State = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut

	state, reason, current, target, classification := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut)
	}
	if reason != "AppliedRevisionPending" {
		t.Fatalf("reason = %q, want AppliedRevisionPending", reason)
	}
	if current == nil || current.ServiceProfileRef == nil || current.ServiceProfileRef.Name != standardV1Name {
		t.Fatalf("current status = %#v, want serviceProfileRef %s", current, standardV1Name)
	}
	if target == nil || target.ServiceProfileRef == nil || target.ServiceProfileRef.Name != standardV2Name {
		t.Fatalf("target status = %#v, want serviceProfileRef %s", target, standardV2Name)
	}
	if classification == nil || classification.Class != openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace {
		t.Fatalf("classification = %#v, want in-place", classification)
	}
}

func TestReconcileRequestState_BlocksWhenTargetRevisionAlreadyApplied(t *testing.T) {
	t.Parallel()

	objects := upgradeCatalogObjects()
	for _, obj := range objects {
		claim, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaim)
		if !ok {
			continue
		}
		claim.Spec.ServiceOfferingRef = nil
		claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: standardV2Name}
		claim.Status.Phase = openbaov1alpha1.OpenBaoClusterClaimPhaseReady
		claim.Status.Rollout.State = openbaov1alpha1.OpenBaoClusterClaimRolloutStateIdle
		claim.Status.Applied.ServiceOfferingRef = nil
		claim.Status.Applied.ServiceProfileRef = &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
			Name: standardV2Name,
			UID:  "standard-v2-uid",
		}
		claim.Status.Applied.ApprovedContract = targetApprovedContractStatus()
	}
	reconciler := newUpgradeTestReconciler(t, objects...)

	request := newUpgradeRequest("upgrade-1", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
		ServiceProfileRef: &openbaov1alpha1.LocalReference{Name: standardV2Name},
	})

	state, reason, _, target, classification := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked)
	}
	if reason != reasonAlreadyApplied {
		t.Fatalf("reason = %q, want %s", reason, reasonAlreadyApplied)
	}
	if target == nil || target.ServiceProfileRef == nil || target.ServiceProfileRef.Name != standardV2Name {
		t.Fatalf("target status = %#v, want serviceProfileRef %s", target, standardV2Name)
	}
	if classification == nil || classification.Class != openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked || classification.Reason != reasonAlreadyApplied {
		t.Fatalf("classification = %#v, want blocked/%s", classification, reasonAlreadyApplied)
	}
}

func TestReconcileRequestState_ObservesPromotedTargetWhenStatusWasLost(t *testing.T) {
	t.Parallel()

	request := newUpgradeRequest("upgrade-recovered", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
		ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
	})
	request.UID = types.UID("upgrade-recovered-uid")
	objects := append(upgradeCatalogObjects(), &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "payments",
			Name:       "payments-bao",
			Generation: 1,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Phase:              openbaov1alpha1.ClusterPhaseRunning,
			ObservedGeneration: 1,
		},
	})
	for _, obj := range objects {
		claim, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaim)
		if !ok {
			continue
		}
		claim.Annotations = map[string]string{
			constants.AnnotationClaimUpgradeRequest: upgradeRequestToken(request),
		}
		claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard"}
		claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: standardV2Name}
		claim.Status.Phase = openbaov1alpha1.OpenBaoClusterClaimPhaseReady
		claim.Status.Rollout.State = openbaov1alpha1.OpenBaoClusterClaimRolloutStateIdle
		claim.Status.Applied.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard"}
		claim.Status.Applied.ServiceProfileRef = &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
			Name: standardV2Name,
			UID:  "standard-v2-uid",
		}
		claim.Status.Applied.ApprovedContract = targetApprovedContractStatus()
	}
	reconciler := newUpgradeTestReconciler(t, objects...)

	state, reason, current, target, classification := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded)
	}
	if reason != reasonUpgradeApplied {
		t.Fatalf("reason = %q, want %s", reason, reasonUpgradeApplied)
	}
	if current == nil || current.ServiceProfileRef == nil || current.ServiceProfileRef.Name != standardV2Name {
		t.Fatalf("current status = %#v, want serviceProfileRef %s", current, standardV2Name)
	}
	if target == nil || target.ServiceProfileRef == nil || target.ServiceProfileRef.Name != standardV2Name {
		t.Fatalf("target status = %#v, want serviceProfileRef %s", target, standardV2Name)
	}
	if classification == nil || classification.Class != openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace {
		t.Fatalf("classification = %#v, want in-place", classification)
	}
}

func TestReconcileRequestState_BlocksWhenAnotherRequestIsActive(t *testing.T) {
	t.Parallel()

	objects := append(upgradeCatalogObjects(), &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "upgrade-1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"},
			Target: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
				ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatus{
			State: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
		},
	})
	reconciler := newUpgradeTestReconciler(t, objects...)

	request := newUpgradeRequest("upgrade-2", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
		ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
	})

	state, reason, current, target, classification := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked)
	}
	if reason != reasonAnotherUpgradeRequestActive {
		t.Fatalf("reason = %q, want %s", reason, reasonAnotherUpgradeRequestActive)
	}
	if current == nil || current.ServiceProfileRef == nil || current.ServiceProfileRef.Name != standardV1Name {
		t.Fatalf("current status = %#v, want serviceProfileRef %s", current, standardV1Name)
	}
	if target != nil {
		t.Fatalf("target status = %#v, want nil", target)
	}
	if classification == nil || classification.Class != openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked || classification.Reason != reasonAnotherUpgradeRequestActive {
		t.Fatalf("classification = %#v, want blocked/%s", classification, reasonAnotherUpgradeRequestActive)
	}
}

func TestReconcileRequestState_SucceedsAfterSameClusterConvergence(t *testing.T) {
	t.Parallel()

	objects := append(upgradeCatalogObjects(), &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "payments",
			Name:       "payments-bao",
			Generation: 1,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Phase:              openbaov1alpha1.ClusterPhaseRunning,
			ObservedGeneration: 1,
		},
	})
	for _, obj := range objects {
		claim, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaim)
		if !ok {
			continue
		}
		claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: standardV2Name}
		claim.Status.Phase = openbaov1alpha1.OpenBaoClusterClaimPhaseReady
		claim.Status.Rollout.State = openbaov1alpha1.OpenBaoClusterClaimRolloutStateIdle
		claim.Status.Applied.ServiceProfileRef = &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
			Name: standardV2Name,
			UID:  "standard-v2-uid",
		}
		claim.Status.Applied.ApprovedContract = targetApprovedContractStatus()
	}
	reconciler := newUpgradeTestReconciler(t, objects...)

	request := newUpgradeRequest("upgrade-1", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
		ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
	})
	request.Status.State = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut

	state, reason, current, target, classification := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded)
	}
	if reason != reasonUpgradeApplied {
		t.Fatalf("reason = %q, want %s", reason, reasonUpgradeApplied)
	}
	if current == nil || current.ServiceProfileRef == nil || current.ServiceProfileRef.Name != standardV2Name {
		t.Fatalf("current status = %#v, want serviceProfileRef %s", current, standardV2Name)
	}
	if target == nil || target.ServiceProfileRef == nil || target.ServiceProfileRef.Name != standardV2Name {
		t.Fatalf("target status = %#v, want serviceProfileRef %s", target, standardV2Name)
	}
	if classification == nil || classification.Class != openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace {
		t.Fatalf("classification = %#v, want in-place", classification)
	}
}

func TestReconcileRequestState_SucceedsWhenClaimIsDegradedButServiceRemainsAvailable(t *testing.T) {
	t.Parallel()

	objects := append(upgradeCatalogObjects(), &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "payments",
			Name:       "payments-bao",
			Generation: 1,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Phase:              openbaov1alpha1.ClusterPhaseRunning,
			ObservedGeneration: 1,
		},
	})
	for _, obj := range objects {
		claim, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaim)
		if !ok {
			continue
		}
		claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: standardV2Name}
		claim.Status.Phase = openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded
		claim.Status.Rollout.State = openbaov1alpha1.OpenBaoClusterClaimRolloutStateIdle
		claim.Status.Applied.ServiceProfileRef = &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
			Name: standardV2Name,
			UID:  "standard-v2-uid",
		}
		claim.Status.Applied.ApprovedContract = targetApprovedContractStatus()
		claim.Status.Conditions = []metav1.Condition{
			{Type: "ServiceAvailable", Status: metav1.ConditionTrue, Reason: "RollingOut"},
			{Type: "MaintenanceActive", Status: metav1.ConditionTrue, Reason: "RollingOut"},
		}
	}
	reconciler := newUpgradeTestReconciler(t, objects...)

	request := newUpgradeRequest("upgrade-1", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
		ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
	})
	request.Status.State = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut

	state, reason, _, _, _ := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded)
	}
	if reason != reasonUpgradeApplied {
		t.Fatalf("reason = %q, want %s", reason, reasonUpgradeApplied)
	}
}

func TestReconcile_DoesNotPollWhileRollingOutWhenStatusIsUnchanged(t *testing.T) {
	t.Parallel()

	reconciler := newUpgradeTestReconciler(t, append(
		upgradeCatalogObjects(),
		&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "payments",
				Name:      "upgrade-1",
			},
			Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
				ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"},
				Target: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
					ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
				},
			},
			Status: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatus{
				ObservedGeneration: 1,
				State:              openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut,
				Reason:             "LocalClusterPending",
				Current:            currentRevisionStatusForTest(standardV1Name, "standard-v1-uid", currentApprovedContractStatus().IdentityHash),
				Target:             currentRevisionStatusForTest(standardV2Name, "standard-v2-uid", targetApprovedContractStatus().IdentityHash),
				Classification: classificationStatus(
					openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace,
					reasonInPlaceSupported,
				),
			},
		},
	)...)

	result, err := reconciler.Reconcile(context.Background(), types.NamespacedName{Namespace: "payments", Name: "upgrade-1"}, logr.Discard())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("Reconcile() requeueAfter = %s, want 0", result.RequeueAfter)
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

func newUpgradeTestReconciler(t *testing.T, objects ...client.Object) runtimeReconciler {
	t.Helper()

	scheme := newTestScheme(t)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&openbaov1alpha1.OpenBaoClusterClaim{},
			&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{},
			&openbaov1alpha1.OpenBaoCluster{},
		).
		WithObjects(objects...).
		Build()
	return runtimeReconciler{
		client:              fakeClient,
		reader:              fakeClient,
		enableServiceClaims: true,
	}
}

func newUpgradeRequest(
	name string,
	target openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest {
	return &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      name,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"},
			Target:   target,
		},
	}
}

func baselineApprovedContractAndCatalog() (*claimcontract.ApprovedServiceContract, *claimcontract.CatalogBundle) {
	return &claimcontract.ApprovedServiceContract{
			Cluster: claimcontract.ApprovedCluster{
				Version:         "2.3.0",
				Voters:          3,
				ReadReplicas:    0,
				SecurityProfile: openbaov1alpha1.ProfileDevelopment,
			},
			Unseal: claimcontract.ApprovedUnseal{
				Mode: claimcontract.UnsealPostureModeManagedStatic,
			},
			Storage: claimcontract.ApprovedStorage{
				PrimarySize:     "10Gi",
				ReadReplicaSize: "10Gi",
			},
			Bootstrap: claimcontract.ApprovedBootstrap{
				Mode:       openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
				ProfileRef: &openbaov1alpha1.LocalReference{Name: "bootstrap-v1"},
			},
			Exposure: claimcontract.ApprovedExposure{
				ClassRef: openbaov1alpha1.LocalReference{Name: "internal-v1"},
			},
			Backup: claimcontract.ApprovedBackup{
				ProfileRef: openbaov1alpha1.LocalReference{Name: "backup-v1"},
				Parameters: claimcontract.ApprovedBackupParameters{
					Location:  "tenant-payments",
					Partition: "daily",
				},
			},
			Lifecycle: claimcontract.ApprovedLifecycle{
				UpgradeStrategy:    openbaov1alpha1.UpdateStrategyRollingUpdate,
				PreUpgradeSnapshot: false,
			},
		}, &claimcontract.CatalogBundle{
			BackupProfile: &openbaov1alpha1.OpenBaoBackupProfile{
				ObjectMeta: objectMeta("backup-v1", "backup-v1-uid"),
				Spec: openbaov1alpha1.OpenBaoBackupProfileSpec{
					Schedule:  "0 0 * * *",
					Retention: &openbaov1alpha1.BackupRetention{MaxCount: 7},
				},
			},
			BackupTarget: &openbaov1alpha1.OpenBaoBackupTarget{
				ObjectMeta: objectMeta("target-v1", "target-v1-uid"),
			},
			BackupBackend: &openbaov1alpha1.OpenBaoBackupBackend{
				ObjectMeta: objectMeta("backend-v1", "backend-v1-uid"),
			},
			BackupAuth: &openbaov1alpha1.OpenBaoBackupAuthProfile{
				ObjectMeta: objectMeta("auth-v1", "auth-v1-uid"),
			},
			TransferProfile: &openbaov1alpha1.OpenBaoTransferProfile{
				ObjectMeta: objectMeta("transfer-v1", "transfer-v1-uid"),
			},
		}
}

func upgradeCatalogObjects() []client.Object {
	readReplicas := int32(0)

	exposureClass := &openbaov1alpha1.OpenBaoExposureClass{
		ObjectMeta: objectMeta("internal-v1", "internal-v1-uid"),
		Spec: openbaov1alpha1.OpenBaoExposureClassSpec{
			PublishMode: openbaov1alpha1.OpenBaoExposurePublishModeClusterInternal,
			HostnamePolicy: openbaov1alpha1.OpenBaoExposureHostnamePolicySpec{
				Mode:         openbaov1alpha1.OpenBaoExposureHostnamePolicyModeGenerated,
				DomainSuffix: "cluster.local",
			},
		},
	}
	backupProfile := &openbaov1alpha1.OpenBaoBackupProfile{
		ObjectMeta: objectMeta("backup-v1", "backup-v1-uid"),
		Spec: openbaov1alpha1.OpenBaoBackupProfileSpec{
			Schedule:  "0 0 * * *",
			Retention: &openbaov1alpha1.BackupRetention{MaxCount: 7},
		},
	}
	currentProfile := &openbaov1alpha1.OpenBaoServiceProfile{
		ObjectMeta: objectMeta(standardV1Name, "standard-v1-uid"),
		Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
			Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
				Version:         "2.3.0",
				Voters:          3,
				ReadReplicas:    &readReplicas,
				SecurityProfile: openbaov1alpha1.ProfileDevelopment,
			},
			Storage: openbaov1alpha1.OpenBaoServiceProfileStorageSpec{
				PrimarySize: "10Gi",
			},
			Bootstrap: openbaov1alpha1.OpenBaoServiceProfileBootstrapSpec{
				Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit,
			},
			Exposure: openbaov1alpha1.OpenBaoServiceProfileExposureSpec{
				ClassRef: openbaov1alpha1.LocalReference{Name: exposureClass.Name},
			},
			Backup: openbaov1alpha1.OpenBaoServiceProfileBackupSpec{
				ProfileRef: openbaov1alpha1.LocalReference{Name: backupProfile.Name},
			},
			Lifecycle: openbaov1alpha1.OpenBaoServiceProfileLifecycleSpec{
				UpgradeStrategy: openbaov1alpha1.UpdateStrategyRollingUpdate,
			},
		},
	}
	targetProfile := currentProfile.DeepCopy()
	targetProfile.Name = standardV2Name
	targetProfile.UID = types.UID("standard-v2-uid")
	targetProfile.Spec.Cluster.Version = "2.4.0"
	offering := &openbaov1alpha1.OpenBaoServiceOffering{
		ObjectMeta: objectMeta("standard", "standard-offering-uid"),
		Spec: openbaov1alpha1.OpenBaoServiceOfferingSpec{
			CurrentRevisionRef: openbaov1alpha1.LocalReference{Name: targetProfile.Name},
		},
	}
	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: objectMetaNamespaced("payments", "payments-bao", "claim-uid"),
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: currentProfile.Name},
			ServiceOfferingRef: &openbaov1alpha1.LocalReference{
				Name: "standard",
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimStatus{
			Materialization: openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
				Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
				LocalRef: &openbaov1alpha1.NamespacedReference{
					Namespace: "payments",
					Name:      "payments-bao",
				},
			},
			Applied: openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
				ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
				ServiceProfileRef: &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
					Name: currentProfile.Name,
					UID:  string(currentProfile.UID),
				},
				ApprovedContract: currentApprovedContractStatus(),
			},
		},
	}

	return []client.Object{claim, currentProfile, targetProfile, exposureClass, backupProfile, offering}
}

func objectMeta(name, uid string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, UID: types.UID(uid)}
}

func currentRevisionStatusForTest(
	profileName string,
	profileUID string,
	identityHash string,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus {
	status := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus{
		ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: standardOfferingName},
		ServiceProfileRef: &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
			Name: profileName,
			UID:  profileUID,
		},
	}
	if identityHash != "" {
		status.ApprovedContract = &openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus{IdentityHash: identityHash}
	}
	return status
}

func objectMetaNamespaced(namespace, name, uid string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: namespace, Name: name, UID: types.UID(uid)}
}

func currentApprovedContractStatus() *openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus {
	contract, _ := baselineApprovedContractAndCatalog()
	return claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(contract))
}

func targetApprovedContractStatus() *openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus {
	contract, _ := baselineApprovedContractAndCatalog()
	contract.Cluster.Version = version240
	return claimcontract.ContractIdentityStatus(claimcontract.IdentityHash(contract))
}
