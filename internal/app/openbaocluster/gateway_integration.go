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

// GatewayIntegrationResult is the controller-facing evaluation result for the
// operator-managed Gateway API contract.
type GatewayIntegrationResult struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

// EvaluateGatewayIntegration validates the operator-managed Gateway API
// prerequisites, controller support, and Route attachment for the selected mode.
func EvaluateGatewayIntegration(
	ctx context.Context,
	deps StatusIntegrationDependencies,
	cluster *openbaov1alpha1.OpenBaoCluster,
) GatewayIntegrationResult {
	manager := networkingmanager.NewManagerWithReader(
		deps.Client,
		deps.APIReader,
		deps.Scheme,
		deps.OperatorNamespace,
		deps.Platform,
	)
	err := manager.ValidateGatewayIntegration(ctx, cluster)

	switch {
	case err == nil:
		return GatewayIntegrationResult{
			Status:  metav1.ConditionTrue,
			Reason:  constants.ReasonGatewayIntegrationReady,
			Message: "Gateway integration prerequisites and Route attachment are ready",
		}
	case errors.Is(err, networkingmanager.ErrGatewayAPIMissing):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayAPIMissing,
			Message: "Gateway API CRDs required for spec.gateway are not installed",
		}
	case errors.Is(err, networkingmanager.ErrGatewayReferenceMissing):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayReferenceMissing,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayClassMissing):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayClassMissing,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayListenerIncompatible):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayListenerIncompatible,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayClassNotAccepted):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayClassNotAccepted,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayVersionUnsupported):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayVersionUnsupported,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayFeatureUnsupported):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayFeatureUnsupported,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayNotProgrammed):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayNotProgrammed,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayRouteNotAccepted):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayRouteNotAccepted,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayRouteReferencesUnresolved):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayRouteReferencesUnresolved,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayClassPending):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonGatewayClassPending,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayCapabilitiesUnknown):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonGatewayCapabilitiesUnknown,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayProgrammingPending):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonGatewayProgrammingPending,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrGatewayRoutePending):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonGatewayRoutePending,
			Message: err.Error(),
		}
	default:
		return GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonUnknown,
			Message: fmt.Sprintf("Failed to evaluate Gateway integration prerequisites: %v", err),
		}
	}
}
