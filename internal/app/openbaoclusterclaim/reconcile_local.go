package openbaoclusterclaim

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
)

func (r runtimeReconciler) reconcileLocalClusterState(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
	localResolved result,
	ownership result,
	desired *openbaov1alpha1.OpenBaoCluster,
	desiredResult result,
	bootstrapInputs claimcontract.SameClusterBootstrapResolvedInputs,
) (*openbaov1alpha1.OpenBaoCluster, bool, error) {
	if localResolved.Valid && ownership.Valid && desiredResult.Valid {
		projectedChanged, err := r.ensureLocalBootstrapProjectedArtifacts(ctx, claim, localTarget, bootstrapInputs)
		if err != nil {
			return nil, false, err
		}
		cluster, ensured, err := r.ensureLocalCluster(ctx, claim, desired)
		if err != nil {
			return nil, false, err
		}
		return cluster, ensured || projectedChanged, nil
	}
	if localResolved.Valid {
		cluster, err := r.loadLocalCluster(ctx, localTarget)
		if err != nil {
			return nil, false, err
		}
		return cluster, false, nil
	}
	return nil, false, nil
}

func (r runtimeReconciler) ensureLocalCluster(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	desired *openbaov1alpha1.OpenBaoCluster,
) (*openbaov1alpha1.OpenBaoCluster, bool, error) {
	if claim == nil || desired == nil {
		return nil, false, nil
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: desired.Namespace,
			Name:      desired.Name,
		},
	}
	if err := r.readClient().Get(ctx, client.ObjectKeyFromObject(cluster), cluster); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, false, fmt.Errorf("get same-cluster OpenBaoCluster %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		created := desired.DeepCopy()
		if created.Labels == nil {
			created.Labels = map[string]string{}
		}
		created.Labels[constants.LabelOpenBaoOwnershipMode] = constants.LabelValueOpenBaoOwnershipClaimManaged
		created.Labels[constants.LabelOpenBaoClaimNamespace] = claim.Namespace
		created.Labels[constants.LabelOpenBaoClaimName] = claim.Name
		if err := r.client.Create(ctx, created); err != nil {
			if apierrors.IsAlreadyExists(err) {
				reloaded := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Namespace: desired.Namespace, Name: desired.Name}}
				if getErr := r.readClient().Get(ctx, client.ObjectKeyFromObject(reloaded), reloaded); getErr != nil {
					return nil, false, fmt.Errorf("get same-cluster OpenBaoCluster after create race %s/%s: %w", desired.Namespace, desired.Name, getErr)
				}
				return r.updateOwnedLocalCluster(ctx, claim, desired, reloaded)
			}
			return nil, false, fmt.Errorf("create same-cluster OpenBaoCluster %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		return created, true, nil
	}
	return r.updateOwnedLocalCluster(ctx, claim, desired, cluster)
}

func classifyLocalClusterReconcileError(err error) (result, bool) {
	if err == nil {
		return result{}, false
	}
	if classified, ok := resultFromError(err); ok {
		return classified, true
	}

	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) {
		return result{}, false
	}

	message := strings.TrimSpace(statusErr.ErrStatus.Message)
	if message == "" {
		message = strings.TrimSpace(err.Error())
	}

	switch {
	case apierrors.IsInvalid(statusErr):
		return result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster concrete OpenBaoCluster projection was rejected by OpenBaoCluster admission: " + trimAdmissionFailureMessage(message),
		}, true
	case apierrors.IsForbidden(statusErr) && strings.Contains(message, "ValidatingAdmissionPolicy"):
		return result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster concrete OpenBaoCluster projection was rejected by OpenBaoCluster admission: " + trimAdmissionFailureMessage(message),
		}, true
	default:
		return result{}, false
	}
}

func trimAdmissionFailureMessage(message string) string {
	message = strings.TrimSpace(message)
	if idx := strings.Index(message, "denied request:"); idx != -1 {
		return strings.TrimSpace(message[idx+len("denied request:"):])
	}
	return message
}

func (r runtimeReconciler) loadLocalCluster(
	ctx context.Context,
	localTarget *openbaov1alpha1.NamespacedReference,
) (*openbaov1alpha1.OpenBaoCluster, error) {
	if localTarget == nil || localTarget.Namespace == "" || localTarget.Name == "" {
		return nil, nil
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	key := client.ObjectKey{Namespace: localTarget.Namespace, Name: localTarget.Name}
	if err := r.readClient().Get(ctx, key, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get same-cluster OpenBaoCluster %s/%s: %w", key.Namespace, key.Name, err)
	}

	return cluster, nil
}

func (r runtimeReconciler) resolveOwnership(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
	localResolved result,
	materializationResult result,
) result {
	if localResolved.Valid && localTarget != nil {
		return r.resolveSameClusterOwnership(ctx, claim, localTarget)
	}
	if materializationResult.Valid {
		return result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Selected materialization path does not require same-cluster ownership validation.",
		}
	}
	if localResolved.Reason == openbaov1alpha1.ReasonFeatureDisabled {
		return localResolved
	}

	return result{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonPending,
		Message: "Ownership readiness is waiting for materialization selection.",
	}
}

func (r runtimeReconciler) resolveSameClusterOwnership(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
) result {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	key := client.ObjectKey{Namespace: localTarget.Namespace, Name: localTarget.Name}
	if err := r.readClient().Get(ctx, key, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return result{
				Valid:   true,
				Reason:  openbaov1alpha1.ReasonAccepted,
				Message: "No conflicting OpenBaoCluster exists at the resolved same-cluster target.",
			}
		}
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			return result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Same-cluster ownership validation is blocked because the controller cannot read the concrete OpenBaoCluster target.",
			}
		}
		if operatorerrors.IsTransientKubernetesAPI(err) {
			return result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonPending,
				Message: "Same-cluster ownership validation could not load the concrete OpenBaoCluster target yet.",
			}
		}
		return result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Same-cluster ownership validation could not load the concrete OpenBaoCluster target yet.",
		}
	}

	return sameClusterExistingOwnershipResult(claim, cluster)
}

func (r runtimeReconciler) updateOwnedLocalCluster(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	desired *openbaov1alpha1.OpenBaoCluster,
	current *openbaov1alpha1.OpenBaoCluster,
) (*openbaov1alpha1.OpenBaoCluster, bool, error) {
	if current == nil {
		return nil, false, nil
	}
	ownership := sameClusterExistingOwnershipResult(claim, current)
	if !ownership.Valid {
		return nil, false, &claimResultError{result: ownership}
	}

	original := current.DeepCopy()
	if current.Labels == nil {
		current.Labels = map[string]string{}
	}
	current.Labels[constants.LabelOpenBaoOwnershipMode] = constants.LabelValueOpenBaoOwnershipClaimManaged
	current.Labels[constants.LabelOpenBaoClaimNamespace] = claim.Namespace
	current.Labels[constants.LabelOpenBaoClaimName] = claim.Name
	current.Spec = desired.Spec
	if reflect.DeepEqual(original.Labels, current.Labels) && reflect.DeepEqual(original.Spec, current.Spec) {
		return current, false, nil
	}
	if err := r.client.Patch(ctx, current, client.MergeFrom(original)); err != nil {
		return nil, false, fmt.Errorf("update same-cluster OpenBaoCluster %s/%s: %w", current.Namespace, current.Name, err)
	}
	return current, true, nil
}

func sameClusterExistingOwnershipResult(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) result {
	mode := cluster.Labels[constants.LabelOpenBaoOwnershipMode]
	switch mode {
	case "", constants.LabelValueOpenBaoOwnershipDirectManaged:
		return result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "A directly-managed OpenBaoCluster already exists at the resolved same-cluster target.",
		}
	case constants.LabelValueOpenBaoOwnershipClaimManaged:
		if cluster.Labels[constants.LabelOpenBaoClaimNamespace] == "" || cluster.Labels[constants.LabelOpenBaoClaimName] == "" {
			return result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Existing claim-managed OpenBaoCluster is missing required ownership labels.",
			}
		}
		if !localClusterOwnedByClaim(claim, cluster) {
			return result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "A claim-managed OpenBaoCluster already exists at the resolved same-cluster target for a different claim.",
			}
		}
		return result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Resolved same-cluster target is already claim-managed by this OpenBaoClusterClaim.",
		}
	default:
		return result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Existing OpenBaoCluster has an unknown ownership mode label.",
		}
	}
}
