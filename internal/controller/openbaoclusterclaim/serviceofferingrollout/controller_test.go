package serviceofferingrollout

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

func TestMapUpgradeRequestToRollout(t *testing.T) {
	t.Parallel()

	reconciler := &OpenBaoServiceOfferingRolloutReconciler{}
	request := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "upgrade-1",
			Labels: map[string]string{
				constants.LabelOpenBaoServiceOfferingRollout: "standard-v2-rollout",
			},
		},
	}

	requests := reconciler.fromUpgradeRequest()(context.Background(), request)
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].NamespacedName != (client.ObjectKey{Name: "standard-v2-rollout"}) {
		t.Fatalf("request key = %#v, want standard-v2-rollout", requests[0].NamespacedName)
	}
}

func TestMapExternalUpgradeRequestToSelectingRollout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newTestScheme(t)
	reconciler := &OpenBaoServiceOfferingRolloutReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&openbaov1alpha1.OpenBaoServiceOfferingRollout{
					ObjectMeta: metav1.ObjectMeta{Name: "standard-rollout"},
					Spec: openbaov1alpha1.OpenBaoServiceOfferingRolloutSpec{
						OfferingRef: openbaov1alpha1.LocalReference{Name: "standard"},
					},
				},
				&openbaov1alpha1.OpenBaoClusterClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "payments",
						Name:      "bao-a",
					},
					Status: openbaov1alpha1.OpenBaoClusterClaimStatus{
						Applied: openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
							ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
						},
					},
				},
			).
			Build(),
		Scheme: scheme,
	}
	request := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "manual-upgrade",
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: "bao-a"},
		},
	}

	requests := reconciler.fromUpgradeRequest()(ctx, request)
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].NamespacedName != (client.ObjectKey{Name: "standard-rollout"}) {
		t.Fatalf("request key = %#v, want standard-rollout", requests[0].NamespacedName)
	}
}

func TestMapClaimToRollouts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newTestScheme(t)
	reconciler := &OpenBaoServiceOfferingRolloutReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&openbaov1alpha1.OpenBaoServiceOfferingRollout{
					ObjectMeta: metav1.ObjectMeta{Name: "standard-rollout"},
					Spec: openbaov1alpha1.OpenBaoServiceOfferingRolloutSpec{
						OfferingRef: openbaov1alpha1.LocalReference{Name: "standard"},
					},
				},
				&openbaov1alpha1.OpenBaoServiceOfferingRollout{
					ObjectMeta: metav1.ObjectMeta{Name: "sensitive-rollout"},
					Spec: openbaov1alpha1.OpenBaoServiceOfferingRolloutSpec{
						OfferingRef: openbaov1alpha1.LocalReference{Name: "sensitive"},
					},
				},
			).
			Build(),
		Scheme: scheme,
	}
	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "bao-a",
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimStatus{
			Applied: openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
				ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
			},
		},
	}

	requests := reconciler.fromClaim()(ctx, claim)
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].NamespacedName != (client.ObjectKey{Name: "standard-rollout"}) {
		t.Fatalf("request key = %#v, want standard-rollout", requests[0].NamespacedName)
	}
}

func TestReconcile(t *testing.T) {
	t.Parallel()

	t.Run("requires app reconciler", func(t *testing.T) {
		t.Parallel()

		reconciler := &OpenBaoServiceOfferingRolloutReconciler{}
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{})
		if err == nil {
			t.Fatal("Reconcile() error = nil, want configuration error")
		}
		if err.Error() != "openbaoserviceofferingrollout app reconciler is not configured" {
			t.Fatalf("Reconcile() error = %q", err.Error())
		}
	})

	t.Run("delegates to app reconciler", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("boom")
		app := &stubAppReconciler{
			result: recon.Result{RequeueAfter: 5},
			err:    wantErr,
		}
		reconciler := &OpenBaoServiceOfferingRolloutReconciler{AppReconciler: app}
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "standard-v2-rollout"}}

		result, err := reconciler.Reconcile(context.Background(), req)
		if !errors.Is(err, wantErr) {
			t.Fatalf("Reconcile() error = %v, want %v", err, wantErr)
		}
		if result.RequeueAfter != app.result.RequeueAfter {
			t.Fatalf("Reconcile() requeueAfter = %v, want %v", result.RequeueAfter, app.result.RequeueAfter)
		}
		if app.key != req.NamespacedName {
			t.Fatalf("app reconciler key = %#v, want %#v", app.key, req.NamespacedName)
		}
	})
}

type stubAppReconciler struct {
	key    types.NamespacedName
	result recon.Result
	err    error
}

func (s *stubAppReconciler) Reconcile(_ context.Context, key types.NamespacedName, _ logr.Logger) (recon.Result, error) {
	s.key = key
	return s.result, s.err
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	return scheme
}
