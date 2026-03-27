package openbaocluster

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
)

// GatewayIntegrationDependencies groups infrastructure readers required to
// evaluate the operator-owned Gateway API contract.
type GatewayIntegrationDependencies struct {
	Client            client.Client
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	OperatorNamespace string
	Platform          string
}

// GatewayIntegrationReasonPolicy configures low-cardinality reasons surfaced by
// the controller for Gateway integration readiness.
type GatewayIntegrationReasonPolicy struct {
	Ready                       string
	Unknown                     string
	GatewayAPIMissing           string
	GatewayReferenceMissing     string
	GatewayClassMissing         string
	GatewayClassPending         string
	GatewayClassNotAccepted     string
	GatewayVersionUnsupported   string
	GatewayFeatureUnsupported   string
	GatewayCapabilitiesUnknown  string
	GatewayNotProgrammed        string
	GatewayProgrammingPending   string
	GatewayListenerIncompatible string
}

// GatewayIntegrationResult is the controller-facing evaluation result for the
// operator-managed Gateway API contract.
type GatewayIntegrationResult struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

func (p GatewayIntegrationReasonPolicy) readyReason() string {
	return fallbackReason(p.Ready, constants.ReasonGatewayIntegrationReady)
}

func (p GatewayIntegrationReasonPolicy) unknownReason() string {
	return fallbackReason(p.Unknown, constants.ReasonUnknown)
}

func (p GatewayIntegrationReasonPolicy) gatewayAPIMissingReason() string {
	return fallbackReason(p.GatewayAPIMissing, constants.ReasonGatewayAPIMissing)
}

func (p GatewayIntegrationReasonPolicy) gatewayReferenceMissingReason() string {
	return fallbackReason(p.GatewayReferenceMissing, constants.ReasonGatewayReferenceMissing)
}

func (p GatewayIntegrationReasonPolicy) gatewayClassMissingReason() string {
	return fallbackReason(p.GatewayClassMissing, constants.ReasonGatewayClassMissing)
}

func (p GatewayIntegrationReasonPolicy) gatewayClassPendingReason() string {
	return fallbackReason(p.GatewayClassPending, constants.ReasonGatewayClassPending)
}

func (p GatewayIntegrationReasonPolicy) gatewayClassNotAcceptedReason() string {
	return fallbackReason(p.GatewayClassNotAccepted, constants.ReasonGatewayClassNotAccepted)
}

func (p GatewayIntegrationReasonPolicy) gatewayVersionUnsupportedReason() string {
	return fallbackReason(p.GatewayVersionUnsupported, constants.ReasonGatewayVersionUnsupported)
}

func (p GatewayIntegrationReasonPolicy) gatewayFeatureUnsupportedReason() string {
	return fallbackReason(p.GatewayFeatureUnsupported, constants.ReasonGatewayFeatureUnsupported)
}

func (p GatewayIntegrationReasonPolicy) gatewayCapabilitiesUnknownReason() string {
	return fallbackReason(p.GatewayCapabilitiesUnknown, constants.ReasonGatewayCapabilitiesUnknown)
}

func (p GatewayIntegrationReasonPolicy) gatewayNotProgrammedReason() string {
	return fallbackReason(p.GatewayNotProgrammed, constants.ReasonGatewayNotProgrammed)
}

func (p GatewayIntegrationReasonPolicy) gatewayProgrammingPendingReason() string {
	return fallbackReason(p.GatewayProgrammingPending, constants.ReasonGatewayProgrammingPending)
}

func (p GatewayIntegrationReasonPolicy) gatewayListenerIncompatibleReason() string {
	return fallbackReason(p.GatewayListenerIncompatible, constants.ReasonGatewayListenerIncompatible)
}

// EvaluateGatewayIntegration validates the operator-managed Gateway API
// prerequisites and controller support for the selected Gateway mode.
func EvaluateGatewayIntegration(
	ctx context.Context,
	deps GatewayIntegrationDependencies,
	reasons GatewayIntegrationReasonPolicy,
	cluster *openbaov1alpha1.OpenBaoCluster,
) GatewayIntegrationResult {
	reader := deps.APIReader
	if reader == nil {
		reader = deps.Client
	}

	manager := inframanager.NewManagerWithReader(
		deps.Client,
		reader,
		deps.Scheme,
		deps.OperatorNamespace,
		"",
		nil,
		deps.Platform,
	)
	err := manager.ValidateGatewayIntegration(ctx, cluster)

	switch {
	case err == nil:
		return GatewayIntegrationResult{
			Status:  metav1.ConditionTrue,
			Reason:  reasons.readyReason(),
			Message: "Gateway integration prerequisites are satisfied",
		}
	case errors.Is(err, inframanager.ErrGatewayAPIMissing):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.gatewayAPIMissingReason(),
			Message: "Gateway API CRDs required for spec.gateway are not installed",
		}
	case errors.Is(err, inframanager.ErrGatewayReferenceMissing):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.gatewayReferenceMissingReason(),
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrGatewayClassMissing):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.gatewayClassMissingReason(),
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrGatewayListenerIncompatible):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.gatewayListenerIncompatibleReason(),
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrGatewayClassNotAccepted):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.gatewayClassNotAcceptedReason(),
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrGatewayVersionUnsupported):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.gatewayVersionUnsupportedReason(),
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrGatewayFeatureUnsupported):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.gatewayFeatureUnsupportedReason(),
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrGatewayNotProgrammed):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.gatewayNotProgrammedReason(),
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrGatewayClassPending):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasons.gatewayClassPendingReason(),
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrGatewayCapabilitiesUnknown):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasons.gatewayCapabilitiesUnknownReason(),
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrGatewayProgrammingPending):
		return GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasons.gatewayProgrammingPendingReason(),
			Message: err.Error(),
		}
	default:
		return GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasons.unknownReason(),
			Message: fmt.Sprintf("Failed to evaluate Gateway integration prerequisites: %v", err),
		}
	}
}
