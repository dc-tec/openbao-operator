package openbaocluster

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	networkingmanager "github.com/dc-tec/openbao-operator/internal/service/networking"
)

// IngressIntegrationResult is the controller-facing evaluation result for the
// operator-managed ingress contract.
type IngressIntegrationResult struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

// EvaluateIngressIntegration validates the operator-managed ingress
// prerequisites and controller support for the selected ingress mode.
func EvaluateIngressIntegration(
	ctx context.Context,
	deps StatusIntegrationDependencies,
	cluster *openbaov1alpha1.OpenBaoCluster,
) IngressIntegrationResult {
	manager := networkingmanager.NewManagerWithReader(
		deps.Client,
		deps.APIReader,
		deps.Scheme,
		deps.OperatorNamespace,
		deps.Platform,
	)
	err := manager.ValidateIngressIntegration(ctx, cluster)

	switch {
	case err == nil:
		return IngressIntegrationResult{
			Status:  metav1.ConditionTrue,
			Reason:  constants.ReasonIngressIntegrationReady,
			Message: "Ingress integration prerequisites are satisfied",
		}
	case errors.Is(err, networkingmanager.ErrIngressClassMissing):
		return IngressIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonIngressClassMissing,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrIngressObjectMissing):
		return IngressIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonIngressObjectPending,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrIngressLoadBalancerPending):
		return IngressIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonIngressLoadBalancerPending,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrIngressCapabilitiesUnknown):
		return IngressIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonIngressCapabilitiesUnknown,
			Message: err.Error(),
		}
	default:
		return IngressIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonUnknown,
			Message: fmt.Sprintf("Failed to evaluate ingress integration prerequisites: %v", err),
		}
	}
}
