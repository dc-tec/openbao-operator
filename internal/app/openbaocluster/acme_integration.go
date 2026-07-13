package openbaocluster

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	networkingmanager "github.com/dc-tec/openbao-operator/internal/service/networking"
)

// ACMEIntegrationResult is the controller-facing evaluation result for the
// operator-managed ACME integration contract.
type ACMEIntegrationResult struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

// EvaluateACMEIntegration validates the operator-managed prerequisites around
// OpenBao's native ACME flow and returns controller-ready status information.
func EvaluateACMEIntegration(
	ctx context.Context,
	deps StatusIntegrationDependencies,
	cluster *openbaov1alpha1.OpenBaoCluster,
) ACMEIntegrationResult {
	manager := networkingmanager.NewManagerWithReader(
		deps.Client,
		deps.APIReader,
		deps.Scheme,
		deps.OperatorNamespace,
		deps.Platform,
	)
	err := manager.ValidateACMEPreflight(ctx, logr.Discard(), cluster)

	switch {
	case err == nil:
		return ACMEIntegrationResult{
			Status:  metav1.ConditionTrue,
			Reason:  constants.ReasonACMEIntegrationReady,
			Message: "ACME integration prerequisites are satisfied",
		}
	case errors.Is(err, networkingmanager.ErrGatewayAPIMissing):
		return ACMEIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayAPIMissing,
			Message: "Gateway API CRDs required for ACME passthrough are not installed",
		}
	case errors.Is(err, networkingmanager.ErrACMEGatewayNotConfiguredForPassthrough):
		return ACMEIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonACMEGatewayNotConfiguredForPassthrough,
			Message: err.Error(),
		}
	case errors.Is(err, networkingmanager.ErrACMEDomainNotResolvable):
		return ACMEIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonACMEDomainNotResolvable,
			Message: err.Error(),
		}
	case operatorerrors.IsPermanent(err):
		return ACMEIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonPrerequisitesMissing,
			Message: err.Error(),
		}
	default:
		return ACMEIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonUnknown,
			Message: fmt.Sprintf("Failed to evaluate ACME integration prerequisites: %v", err),
		}
	}
}
