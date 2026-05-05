package openbaoclusterclaim

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestEmitClaimEventsPublishesUserFacingTransitions(t *testing.T) {
	t.Parallel()

	recorder := events.NewFakeRecorder(4)
	reconciler := runtimeReconciler{recorder: recorder}
	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "bao"},
		Status: openbaov1alpha1.OpenBaoClusterClaimStatus{
			Phase: openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
			Materialization: openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
				LocalRef: &openbaov1alpha1.NamespacedReference{Namespace: "tenant-payments", Name: "bao"},
			},
			Conditions: []metav1.Condition{
				{
					Type:               conditionTypeAccepted,
					Status:             metav1.ConditionTrue,
					Reason:             string(openbaov1alpha1.ReasonAccepted),
					Message:            "Claim accepted by tenant governance.",
					ObservedGeneration: 1,
				},
				{
					Type:               conditionTypeMaterialization,
					Status:             metav1.ConditionTrue,
					Reason:             string(openbaov1alpha1.ReasonAccepted),
					Message:            "Materialization resolved.",
					ObservedGeneration: 1,
				},
			},
		},
	}

	reconciler.emitClaimEvents(&openbaov1alpha1.OpenBaoClusterClaim{}, claim)

	expectRecordedEventContains(t, recorder, "Normal", reasonClaimAccepted)
	expectRecordedEventContains(t, recorder, "Normal", reasonClaimMaterialized, "tenant-payments/bao")
	expectRecordedEventContains(t, recorder, "Normal", reasonClaimReady)
}

func expectRecordedEventContains(t *testing.T, recorder *events.FakeRecorder, parts ...string) {
	t.Helper()

	select {
	case event := <-recorder.Events:
		for _, part := range parts {
			if !strings.Contains(event, part) {
				t.Fatalf("event %q does not contain %q", event, part)
			}
		}
	default:
		t.Fatalf("expected event containing %q, got none", strings.Join(parts, ", "))
	}
}
