package openbaoclusterclaim

import (
	"context"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
)

func (r runtimeReconciler) reconcileDeletion(ctx context.Context, claim *openbaov1alpha1.OpenBaoClusterClaim) (recon.Result, error) {
	if !hasFinalizer(claim.Finalizers, openbaov1alpha1.OpenBaoClusterClaimFinalizer) {
		return recon.Result{}, nil
	}

	localPending, err := r.reconcileLocalDeletion(ctx, claim)
	if err != nil {
		return recon.Result{}, err
	}
	if localPending {
		return recon.Result{}, nil
	}

	original := claim.DeepCopy()
	claim.Finalizers = removeFinalizer(claim.Finalizers, openbaov1alpha1.OpenBaoClusterClaimFinalizer)
	if reflect.DeepEqual(claim.Finalizers, original.Finalizers) {
		return recon.Result{}, nil
	}
	if err := r.client.Patch(ctx, claim, client.MergeFrom(original)); err != nil {
		return recon.Result{}, fmt.Errorf("remove OpenBaoClusterClaim finalizer: %w", err)
	}

	return recon.Result{}, nil
}

func (r runtimeReconciler) reconcileLocalDeletion(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) (bool, error) {
	localTarget, err := r.resolveDeletionLocalTarget(ctx, claim)
	if err != nil {
		return false, err
	}
	if localTarget == nil {
		return false, nil
	}

	cluster, err := r.loadLocalCluster(ctx, localTarget)
	if err != nil {
		return false, fmt.Errorf("get same-cluster OpenBaoCluster during claim deletion %s/%s: %w", localTarget.Namespace, localTarget.Name, err)
	}
	if cluster != nil {
		if !localClusterOwnedByClaim(claim, cluster) {
			return false, nil
		}
		if !cluster.DeletionTimestamp.IsZero() {
			return true, nil
		}
		if err := r.client.Delete(ctx, cluster); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("delete same-cluster OpenBaoCluster during claim deletion %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}

		reloaded, err := r.loadLocalCluster(ctx, localTarget)
		if err != nil {
			return false, fmt.Errorf("get same-cluster OpenBaoCluster after delete %s/%s: %w", localTarget.Namespace, localTarget.Name, err)
		}
		if reloaded != nil {
			return true, nil
		}
	}

	err = r.deleteLocalBootstrapProjectedArtifacts(
		ctx,
		claim,
		localTarget.Namespace,
		bootstrapProjectionRefsForDeletion(claim, cluster),
	)
	if err != nil {
		return false, err
	}
	return false, nil
}

func (r runtimeReconciler) resolveDeletionLocalTarget(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) (*openbaov1alpha1.NamespacedReference, error) {
	if claim == nil {
		return nil, nil
	}
	if claim.Status.Materialization.LocalRef != nil &&
		claim.Status.Materialization.LocalRef.Namespace != "" &&
		claim.Status.Materialization.LocalRef.Name != "" {
		return &openbaov1alpha1.NamespacedReference{
			Namespace: claim.Status.Materialization.LocalRef.Namespace,
			Name:      claim.Status.Materialization.LocalRef.Name,
		}, nil
	}
	if claim.Spec.TenantRef.Name == "" {
		return nil, nil
	}

	tenant := &openbaov1alpha1.OpenBaoTenant{}
	key := client.ObjectKey{Namespace: claim.Namespace, Name: claim.Spec.TenantRef.Name}
	if err := r.client.Get(ctx, key, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get OpenBaoTenant during claim deletion %s/%s: %w", key.Namespace, key.Name, err)
	}
	if tenant.Spec.TargetNamespace == "" {
		return nil, nil
	}

	return &openbaov1alpha1.NamespacedReference{
		Namespace: tenant.Spec.TargetNamespace,
		Name:      claimcontract.ClaimManagedLocalClusterName(claim.Name),
	}, nil
}

func localClusterOwnedByClaim(claim *openbaov1alpha1.OpenBaoClusterClaim, cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if claim == nil || cluster == nil {
		return false
	}
	return cluster.Labels[constants.LabelOpenBaoOwnershipMode] == constants.LabelValueOpenBaoOwnershipClaimManaged &&
		cluster.Labels[constants.LabelOpenBaoClaimNamespace] == claim.Namespace &&
		cluster.Labels[constants.LabelOpenBaoClaimName] == claim.Name
}

func hasFinalizer(finalizers []string, value string) bool {
	for _, finalizer := range finalizers {
		if finalizer == value {
			return true
		}
	}
	return false
}

func ensureFinalizer(obj client.Object, value string) bool {
	if hasFinalizer(obj.GetFinalizers(), value) {
		return false
	}
	obj.SetFinalizers(append(obj.GetFinalizers(), value))
	return true
}

func removeFinalizer(finalizers []string, value string) []string {
	result := make([]string, 0, len(finalizers))
	for _, finalizer := range finalizers {
		if finalizer != value {
			result = append(result, finalizer)
		}
	}
	return result
}
