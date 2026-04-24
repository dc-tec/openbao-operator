package openbaoclusterclaim

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/service/connectionpublishing"
)

func (r runtimeReconciler) reconcileConnectionContract(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localResolved result,
	localCluster *openbaov1alpha1.OpenBaoCluster,
) (bool, connectionpublishing.PublicationResult, error) {
	if localResolved.Valid {
		return r.reconcileLocalConnectionContract(ctx, claim, localCluster)
	}
	publication := connectionpublishing.PublicationResult{
		Publishable: false,
		Reason:      localResolved.Reason,
		Message:     localResolved.Message,
	}
	changed, err := r.deleteConnectionSecret(ctx, claim)
	if err != nil {
		return false, publication, err
	}
	if clearPublishedConnectionStatus(claim) {
		changed = true
	}
	return changed, publication, nil
}

func (r runtimeReconciler) reconcileLocalConnectionContract(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localCluster *openbaov1alpha1.OpenBaoCluster,
) (bool, connectionpublishing.PublicationResult, error) {
	publicService, caSecret, shouldRequeue, err := r.loadLocalConnectionInputs(ctx, localCluster)
	if err != nil {
		return false, connectionpublishing.PublicationResult{}, err
	}
	publication := connectionpublishing.EvaluateLocalPublication(localCluster, publicService, caSecret)
	publication.ShouldRequeue = shouldRequeue
	if !publication.Publishable {
		changed, err := r.deleteConnectionSecret(ctx, claim)
		if err != nil {
			return false, publication, err
		}
		if clearPublishedConnectionStatus(claim) {
			changed = true
		}
		return changed, publication, nil
	}

	secret := connectionpublishing.DesiredLocalSecret(claim, localCluster, publicService, caSecret)
	if err := r.applyConnectionSecret(ctx, claim, secret); err != nil {
		if classified, ok := resultFromError(err); ok {
			changed := clearPublishedConnectionStatus(claim)
			return changed, connectionpublishing.PublicationResult{
				Reason:  classified.Reason,
				Message: classified.Message,
			}, nil
		}
		return false, publication, err
	}

	desiredConnection := connectionpublishing.DesiredLocalClaimConnection(claim, localCluster, publicService, caSecret)
	desiredConnection = stampedPublishedConnectionStatus(claim.Status.Connection, desiredConnection)
	if reflect.DeepEqual(claim.Status.Connection, desiredConnection) {
		return false, publication, nil
	}
	claim.Status.Connection = desiredConnection
	return true, publication, nil
}

func stampedPublishedConnectionStatus(
	current openbaov1alpha1.OpenBaoClusterClaimConnectionStatus,
	desired openbaov1alpha1.OpenBaoClusterClaimConnectionStatus,
) openbaov1alpha1.OpenBaoClusterClaimConnectionStatus {
	if desired.Endpoint == "" {
		return desired
	}
	if samePublishedConnectionContract(current, desired) && current.ObservedAt != nil {
		desired.ObservedAt = current.ObservedAt.DeepCopy()
		return desired
	}
	now := metav1.Now()
	desired.ObservedAt = &now
	return desired
}

func samePublishedConnectionContract(
	left openbaov1alpha1.OpenBaoClusterClaimConnectionStatus,
	right openbaov1alpha1.OpenBaoClusterClaimConnectionStatus,
) bool {
	return left.Endpoint == right.Endpoint &&
		sameLocalReference(left.SecretRef, right.SecretRef) &&
		sameTypedObjectReference(left.CABundleRef, right.CABundleRef)
}

func sameLocalReference(left, right *openbaov1alpha1.LocalReference) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Name == right.Name
}

func sameTypedObjectReference(left, right *openbaov1alpha1.TypedObjectReference) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Kind == right.Kind && left.Name == right.Name && left.Namespace == right.Namespace
}

func (r runtimeReconciler) loadLocalConnectionInputs(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (*corev1.Service, *corev1.Secret, bool, error) {
	if cluster == nil {
		return nil, nil, false, nil
	}

	shouldRequeue := false
	service := &corev1.Service{}
	serviceKey := client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      connectionpublishing.LocalPublicServiceName(cluster.Name),
	}
	if err := r.client.Get(ctx, serviceKey, service); err != nil {
		switch {
		case apierrors.IsNotFound(err):
			service = nil
		case apierrors.IsForbidden(err), operatorerrors.IsTransientKubernetesAPI(err):
			service = nil
			shouldRequeue = true
		default:
			return nil, nil, false, fmt.Errorf("get local public Service %s/%s: %w", serviceKey.Namespace, serviceKey.Name, err)
		}
	}

	caSecret := &corev1.Secret{}
	caKey := client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      connectionpublishing.LocalCASecretName(cluster.Name),
	}
	if err := r.client.Get(ctx, caKey, caSecret); err != nil {
		switch {
		case apierrors.IsNotFound(err):
			caSecret = nil
		case apierrors.IsForbidden(err), operatorerrors.IsTransientKubernetesAPI(err):
			caSecret = nil
			shouldRequeue = true
		default:
			return nil, nil, false, fmt.Errorf("get local TLS CA Secret %s/%s: %w", caKey.Namespace, caKey.Name, err)
		}
	}

	return service, caSecret, shouldRequeue, nil
}

func (r runtimeReconciler) applyConnectionSecret(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	secret *corev1.Secret,
) error {
	if r.scheme == nil {
		return fmt.Errorf("scheme is required for claim connection publishing")
	}
	if err := r.validateConnectionSecretCustody(ctx, claim, secret); err != nil {
		return err
	}
	if err := applySecretWithFallback(ctx, r.client, r.scheme, claim, secret); err != nil {
		return fmt.Errorf("apply claim connection Secret %s/%s: %w", secret.Namespace, secret.Name, err)
	}
	return nil
}

func (r runtimeReconciler) deleteConnectionSecret(ctx context.Context, claim *openbaov1alpha1.OpenBaoClusterClaim) (bool, error) {
	key := client.ObjectKey{Namespace: claim.Namespace, Name: connectionpublishing.SecretName(claim.Name)}
	secret := &corev1.Secret{}
	if err := r.readClient().Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get claim connection Secret %s/%s before delete: %w", key.Namespace, key.Name, err)
	}
	if !connectionSecretOwnedByClaim(secret, claim) {
		return false, nil
	}
	if err := r.client.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("delete claim connection Secret %s/%s: %w", key.Namespace, key.Name, err)
	}
	return true, nil
}

func (r runtimeReconciler) validateConnectionSecretCustody(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	secret *corev1.Secret,
) error {
	if claim == nil || secret == nil {
		return nil
	}
	current := &corev1.Secret{}
	key := client.ObjectKeyFromObject(secret)
	if err := r.readClient().Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get claim connection Secret %s/%s: %w", key.Namespace, key.Name, err)
	}
	if !connectionSecretOwnedByClaim(current, claim) {
		return invalidResultError(conflictingConnectionSecretMessage(current))
	}
	return nil
}

func clearPublishedConnectionStatus(claim *openbaov1alpha1.OpenBaoClusterClaim) bool {
	if claim == nil {
		return false
	}
	if reflect.DeepEqual(claim.Status.Connection, openbaov1alpha1.OpenBaoClusterClaimConnectionStatus{}) {
		return false
	}
	claim.Status.Connection = openbaov1alpha1.OpenBaoClusterClaimConnectionStatus{}
	return true
}
