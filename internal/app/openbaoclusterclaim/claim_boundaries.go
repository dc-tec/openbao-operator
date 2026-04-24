package openbaoclusterclaim

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

type claimResultError struct {
	result result
}

func (e *claimResultError) Error() string {
	if e == nil {
		return ""
	}
	return e.result.Message
}

func invalidResultError(message string) error {
	return &claimResultError{
		result: result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: message,
		},
	}
}

func resultFromError(err error) (result, bool) {
	var target *claimResultError
	if errors.As(err, &target) && target != nil {
		return target.result, true
	}
	return result{}, false
}

func (r runtimeReconciler) readClient() client.Reader {
	if r.reader != nil {
		return r.reader
	}
	return r.client
}

func bootstrapProjectionObjectOwnedByClaim(obj client.Object, claim *openbaov1alpha1.OpenBaoClusterClaim) bool {
	if obj == nil || claim == nil {
		return false
	}
	labels := obj.GetLabels()
	return labels[constants.LabelAppManagedBy] == constants.LabelValueAppManagedByOpenBaoOperator &&
		labels[constants.LabelOpenBaoOwnershipMode] == constants.LabelValueOpenBaoOwnershipClaimManaged &&
		labels[constants.LabelOpenBaoClaimNamespace] == claim.Namespace &&
		labels[constants.LabelOpenBaoClaimName] == claim.Name &&
		labels[constants.LabelOpenBaoComponent] == bootstrapProjectionComponent
}

func connectionSecretOwnedByClaim(secret client.Object, claim *openbaov1alpha1.OpenBaoClusterClaim) bool {
	if secret == nil || claim == nil {
		return false
	}
	labels := secret.GetLabels()
	return metav1.IsControlledBy(secret, claim) &&
		labels[constants.LabelAppManagedBy] == constants.LabelValueAppManagedByOpenBaoOperator &&
		labels[constants.LabelOpenBaoClaimNamespace] == claim.Namespace &&
		labels[constants.LabelOpenBaoClaimName] == claim.Name &&
		labels[connectionContractLabelKey] == connectionContractLabelValue
}

func conflictingConnectionSecretMessage(secret client.Object) string {
	return fmt.Sprintf(
		"Claim connection publication is blocked because Secret %s/%s already exists and is not owned by this OpenBaoClusterClaim.",
		secret.GetNamespace(),
		secret.GetName(),
	)
}

func conflictingBootstrapArtifactMessage(kind, namespace, name string) string {
	return fmt.Sprintf(
		"Same-cluster bootstrap projection is blocked because %s %s/%s already exists and is not managed by this OpenBaoClusterClaim.",
		kind,
		namespace,
		name,
	)
}
