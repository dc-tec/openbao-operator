package openbaoclusterclaim

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
	"github.com/dc-tec/openbao-operator/internal/service/connectionpublishing"
)

func TestResolveActiveUpgradeRequestReturnsEarliestNonTerminalRequest(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	requestTime := time.Date(2026, time.April, 23, 10, 0, 0, 0, time.UTC)

	terminal := newClaimUpgradeRequestFixture("payments-bao-upgrade-terminal", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded, "UpgradeCompleted")
	terminal.CreationTimestamp = metav1.NewTime(requestTime.Add(-2 * time.Minute))

	activeOlder := newClaimUpgradeRequestFixture("payments-bao-upgrade-1", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut, "AppliedRevisionPending")
	activeOlder.CreationTimestamp = metav1.NewTime(requestTime)

	activeNewer := newClaimUpgradeRequestFixture("payments-bao-upgrade-2", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut, "AppliedRevisionPending")
	activeNewer.CreationTimestamp = metav1.NewTime(requestTime.Add(time.Minute))

	otherClaim := newClaimUpgradeRequestFixture("other-upgrade", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut, "AppliedRevisionPending")
	otherClaim.Spec.ClaimRef.Name = "other-bao"

	_, builder := newClaimTestClientBuilder(t)
	c := builder.WithObjects(claim.DeepCopy(), terminal, activeOlder, activeNewer, otherClaim).Build()
	reconciler := runtimeReconciler{reader: c}

	request, err := reconciler.resolveActiveUpgradeRequest(context.Background(), claim)
	if err != nil {
		t.Fatalf("resolveActiveUpgradeRequest() error = %v", err)
	}
	if request == nil {
		t.Fatal("resolveActiveUpgradeRequest() = nil, want earliest non-terminal request")
	}
	if request.Name != activeOlder.Name {
		t.Fatalf("resolveActiveUpgradeRequest() name = %q, want %q", request.Name, activeOlder.Name)
	}
}

func TestDesiredUpgradeStatusDefaultsUnreconciledRequestToPending(t *testing.T) {
	t.Parallel()

	request := newClaimUpgradeRequestFixture("payments-bao-upgrade-1", "", "")
	request.Status.Classification = nil

	status := desiredUpgradeStatus(request)
	if status == nil {
		t.Fatal("desiredUpgradeStatus() = nil, want summary")
	}
	if status.RequestRef == nil || status.RequestRef.Name != request.Name {
		t.Fatalf("desiredUpgradeStatus() requestRef = %#v, want %q", status.RequestRef, request.Name)
	}
	if status.State != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending {
		t.Fatalf("desiredUpgradeStatus() state = %q, want %q", status.State, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending)
	}
}

func TestReconcileClaimPublishesActiveUpgradeSummary(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	upgrade := newClaimUpgradeRequestFixture("payments-bao-upgrade-1", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut, "AppliedRevisionPending")

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+3)
	objects = append(objects, claim.DeepCopy(), validTenant(), upgrade)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Upgrade == nil {
		t.Fatal("claim status upgrade = nil, want active workflow summary")
	}
	if updated.Status.Upgrade.RequestRef == nil || updated.Status.Upgrade.RequestRef.Name != upgrade.Name {
		t.Fatalf("claim status upgrade requestRef = %#v, want %q", updated.Status.Upgrade.RequestRef, upgrade.Name)
	}
	if updated.Status.Upgrade.State != openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut {
		t.Fatalf("claim status upgrade state = %q, want %q", updated.Status.Upgrade.State, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut)
	}
	if updated.Status.Upgrade.Reason != "AppliedRevisionPending" {
		t.Fatalf("claim status upgrade reason = %q, want AppliedRevisionPending", updated.Status.Upgrade.Reason)
	}
	if updated.Status.Upgrade.Classification == nil {
		t.Fatal("claim status upgrade classification = nil, want copied request classification")
	}
	if updated.Status.Upgrade.Classification.Class != openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace {
		t.Fatalf("claim status upgrade classification class = %q, want %q", updated.Status.Upgrade.Classification.Class, openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace)
	}
}

func TestReconcileClaimMarksAvailableServiceAsDegradedDuringActiveUpgrade(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
		Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
		LocalRef: &openbaov1alpha1.NamespacedReference{
			Namespace: "payments",
			Name:      "payments-bao",
		},
	}

	localCluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-bao",
			Namespace: "payments",
			Labels: map[string]string{
				constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
				constants.LabelOpenBaoClaimNamespace: "payments",
				constants.LabelOpenBaoClaimName:      "payments-bao",
			},
		},
		Spec:   validExistingSameClusterConcreteSpec(),
		Status: openbaov1alpha1.OpenBaoClusterStatus{Phase: openbaov1alpha1.ClusterPhaseRunning},
	}

	upgrade := newClaimUpgradeRequestFixture("payments-bao-upgrade-1", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut, "AppliedRevisionPending")

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+6)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		localCluster,
		validSameClusterPublicService(),
		validSameClusterCASecret(),
		upgrade,
	)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded {
		t.Fatalf("claim status phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded)
	}
	if updated.Status.Summary == nil {
		t.Fatal("claim summary = nil, want active maintenance summary")
	}
	if updated.Status.Summary.Severity != openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo {
		t.Fatalf("claim summary severity = %q, want %q", updated.Status.Summary.Severity, openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo)
	}
	if updated.Status.Summary.Reason != string(openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut) {
		t.Fatalf("claim summary reason = %q, want %q", updated.Status.Summary.Reason, openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut)
	}
	if updated.Status.Summary.SourceRef == nil || updated.Status.Summary.SourceRef.Kind != "OpenBaoClusterClaimUpgradeRequest" || updated.Status.Summary.SourceRef.Name != upgrade.Name {
		t.Fatalf("claim summary sourceRef = %#v, want active upgrade request", updated.Status.Summary.SourceRef)
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeServiceAvailable, metav1.ConditionTrue, string(openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut))
	assertCondition(t, updated.Status.Conditions, conditionTypeMaintenanceActive, metav1.ConditionTrue, string(openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut))
	assertCondition(t, updated.Status.Conditions, conditionTypeConnectionPublished, metav1.ConditionTrue, string(openbaov1alpha1.ReasonReady))

	connectionSecret := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: claim.Namespace, Name: connectionpublishing.SecretName(claim.Name)}, connectionSecret); err != nil {
		t.Fatalf("get claim connection Secret error = %v", err)
	}
}

func TestReconcileClaimOmitsTerminalUpgradeSummary(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	upgrade := newClaimUpgradeRequestFixture("payments-bao-upgrade-1", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded, "UpgradeCompleted")

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+3)
	objects = append(objects, claim.DeepCopy(), validTenant(), upgrade)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Upgrade != nil {
		t.Fatalf("claim status upgrade = %#v, want nil once the request is terminal", updated.Status.Upgrade)
	}
}

func TestValidateMaterializedServiceSelectionChange_AllowsActiveInPlaceUpgradeRequest(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Status.Materialization.Mode = openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster
	claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v2"}
	claim.Status.Applied.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
	claim.Status.Applied.ServiceProfileRef = &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{Name: "standard-ha-v1", UID: "standard-ha-v1-uid"}

	request := newClaimUpgradeRequestFixture("payments-bao-upgrade-1", openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut, "AppliedRevisionPending")
	request.Namespace = claim.Namespace
	request.Spec.ClaimRef.Name = claim.Name
	request.Status.Target = &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestRevisionStatus{
		ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard-ha"},
		ServiceProfileRef:  &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{Name: "standard-ha-v2"},
	}

	got := validateMaterializedServiceSelectionChange(claim, request)
	if !got.Valid {
		t.Fatalf("validateMaterializedServiceSelectionChange() = %#v, want valid", got)
	}
}

func TestDesiredAppliedStatusClearsOfferingRefForDirectProfileUpgrade(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Spec.ServiceOfferingRef = nil
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v1"}

	current := *validSameClusterAppliedStatusWithStandardOffering()
	approved, validation := claimcontract.BindApprovedServiceContract(claim, sameClusterCatalogBundle())
	if !validation.Valid || approved == nil {
		t.Fatalf("BindApprovedServiceContract() validation = %#v, approved = %#v", validation, approved)
	}

	got := desiredAppliedStatus(current, claim, approved, nil, result{Valid: true}, result{Valid: false})
	if got.ServiceOfferingRef != nil {
		t.Fatalf("ServiceOfferingRef = %#v, want nil for direct serviceProfileRef claim", got.ServiceOfferingRef)
	}
}

func TestValidateMaterializedServiceSelectionChange_BlocksDirectSelectorMutationWithoutUpgradeRequest(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Status.Materialization.Mode = openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster
	claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v2"}
	claim.Status.Applied.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
	claim.Status.Applied.ServiceProfileRef = &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{Name: "standard-ha-v1", UID: "standard-ha-v1-uid"}

	got := validateMaterializedServiceSelectionChange(claim, nil)
	if got.Valid {
		t.Fatalf("validateMaterializedServiceSelectionChange() = %#v, want invalid", got)
	}
	if got.Reason != openbaov1alpha1.ReasonInvalid {
		t.Fatalf("reason = %q, want %q", got.Reason, openbaov1alpha1.ReasonInvalid)
	}
}

func newClaimUpgradeRequestFixture(
	name string,
	state openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState,
	reason string,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest {
	return &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      name,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: "payments-bao"},
			Target: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestTargetSpec{
				ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard-ha"},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatus{
			State:  state,
			Reason: reason,
			Classification: &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestClassificationStatus{
				Class:  openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace,
				Reason: "InPlaceSupported",
			},
		},
	}
}
