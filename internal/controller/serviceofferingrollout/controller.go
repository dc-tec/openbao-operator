package serviceofferingrollout

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	approllout "github.com/dc-tec/openbao-operator/internal/app/serviceofferingrollout"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	ctrl "sigs.k8s.io/controller-runtime"
)

const controllerNameOpenBaoServiceOfferingRollout = "openbaoserviceofferingrollout"

type OpenBaoServiceOfferingRolloutReconciler struct {
	client.Client
	Reader              client.Reader
	Scheme              *runtime.Scheme
	EnableServiceClaims bool
	AppReconciler       approllout.Reconciler
}

func (r *OpenBaoServiceOfferingRolloutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.AppReconciler == nil {
		return ctrl.Result{}, fmt.Errorf("openbaoserviceofferingrollout app reconciler is not configured")
	}
	result, err := r.AppReconciler.Reconcile(ctx, req.NamespacedName, log.FromContext(ctx).WithName(controllerNameOpenBaoServiceOfferingRollout))
	return ctrl.Result{RequeueAfter: result.RequeueAfter}, err
}

func (r *OpenBaoServiceOfferingRolloutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	if r.Scheme == nil {
		r.Scheme = mgr.GetScheme()
	}
	if r.Reader == nil {
		r.Reader = mgr.GetAPIReader()
	}
	if r.AppReconciler == nil {
		r.AppReconciler = approllout.NewReconciler(approllout.Runtime{
			Client:              r.Client,
			Reader:              r.Reader,
			EnableServiceClaims: r.EnableServiceClaims,
		})
	}
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoServiceOfferingRollout{}).
		Named(controllerNameOpenBaoServiceOfferingRollout)
	if r.EnableServiceClaims {
		builder = builder.Watches(
			&openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{},
			handler.EnqueueRequestsFromMapFunc(r.fromUpgradeRequest()),
		).Watches(
			&openbaov1alpha1.OpenBaoClusterClaim{},
			handler.EnqueueRequestsFromMapFunc(r.fromClaim()),
		).Watches(
			&openbaov1alpha1.OpenBaoServiceOffering{},
			handler.EnqueueRequestsFromMapFunc(r.fromServiceOffering()),
		).Watches(
			&openbaov1alpha1.OpenBaoServiceProfile{},
			handler.EnqueueRequestsFromMapFunc(r.fromServiceProfile()),
		)
	}
	return builder.Complete(r)
}

func (r *OpenBaoServiceOfferingRolloutReconciler) fromUpgradeRequest() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		name := obj.GetLabels()[constants.LabelOpenBaoServiceOfferingRollout]
		if name == "" {
			request, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest)
			if !ok || request == nil || request.Spec.ClaimRef.Name == "" {
				return nil
			}
			claim := &openbaov1alpha1.OpenBaoClusterClaim{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: request.Spec.ClaimRef.Name}, claim); err != nil {
				return nil
			}
			return r.requestsForClaim(ctx, claim)
		}
		return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: name}}}
	}
}

func (r *OpenBaoServiceOfferingRolloutReconciler) fromClaim() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		claim, ok := obj.(*openbaov1alpha1.OpenBaoClusterClaim)
		if !ok || claim == nil {
			return nil
		}

		return r.requestsForClaim(ctx, claim)
	}
}

func (r *OpenBaoServiceOfferingRolloutReconciler) fromServiceOffering() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		offering, ok := obj.(*openbaov1alpha1.OpenBaoServiceOffering)
		if !ok || offering == nil {
			return nil
		}
		return r.rolloutsReferencing(ctx, func(rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout) bool {
			return rollout.Spec.OfferingRef.Name == offering.Name
		})
	}
}

func (r *OpenBaoServiceOfferingRolloutReconciler) fromServiceProfile() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		profile, ok := obj.(*openbaov1alpha1.OpenBaoServiceProfile)
		if !ok || profile == nil {
			return nil
		}
		return r.rolloutsReferencing(ctx, func(rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout) bool {
			return rollout.Spec.TargetRevisionRef.Name == profile.Name
		})
	}
}

func (r *OpenBaoServiceOfferingRolloutReconciler) rolloutsReferencing(
	ctx context.Context,
	matches func(*openbaov1alpha1.OpenBaoServiceOfferingRollout) bool,
) []reconcile.Request {
	rollouts := &openbaov1alpha1.OpenBaoServiceOfferingRolloutList{}
	if err := r.List(ctx, rollouts); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(rollouts.Items))
	for i := range rollouts.Items {
		rollout := &rollouts.Items[i]
		if matches(rollout) {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: rollout.Name}})
		}
	}
	return requests
}

func (r *OpenBaoServiceOfferingRolloutReconciler) requestsForClaim(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) []reconcile.Request {
	rollouts := &openbaov1alpha1.OpenBaoServiceOfferingRolloutList{}
	if err := r.List(ctx, rollouts); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(rollouts.Items))
	for i := range rollouts.Items {
		rollout := &rollouts.Items[i]
		if rolloutSelectsClaim(rollout, claim) {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: rollout.Name}})
		}
	}
	return requests
}

func rolloutSelectsClaim(
	rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) bool {
	if rollout == nil || claim == nil || !rolloutNamespaceSelected(rollout, claim.Namespace) {
		return false
	}
	if claim.Status.Applied.ServiceOfferingRef != nil && claim.Status.Applied.ServiceOfferingRef.Name == rollout.Spec.OfferingRef.Name {
		return true
	}
	return claim.Spec.ServiceOfferingRef != nil && claim.Spec.ServiceOfferingRef.Name == rollout.Spec.OfferingRef.Name
}

func rolloutNamespaceSelected(rollout *openbaov1alpha1.OpenBaoServiceOfferingRollout, namespace string) bool {
	if rollout.Spec.Selector == nil || len(rollout.Spec.Selector.Namespaces) == 0 {
		return true
	}
	for _, selected := range rollout.Spec.Selector.Namespaces {
		if selected == namespace {
			return true
		}
	}
	return false
}
