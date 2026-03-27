package openbaocluster

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
)

// ACMEIntegrationDependencies groups infrastructure readers required to evaluate
// the operator-owned prerequisites around OpenBao's native ACME flow.
type ACMEIntegrationDependencies struct {
	Client            client.Client
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	OperatorNamespace string
	Platform          string
}

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
	deps ACMEIntegrationDependencies,
	cluster *openbaov1alpha1.OpenBaoCluster,
) ACMEIntegrationResult {
	manager := inframanager.NewManagerWithReaderAndOIDCConfig(
		deps.Client,
		deps.APIReader,
		deps.Scheme,
		deps.OperatorNamespace,
		nil,
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
	case errors.Is(err, inframanager.ErrGatewayAPIMissing):
		return ACMEIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGatewayAPIMissing,
			Message: "Gateway API CRDs required for ACME passthrough are not installed",
		}
	case errors.Is(err, inframanager.ErrACMEGatewayNotConfiguredForPassthrough):
		return ACMEIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonACMEGatewayNotConfiguredForPassthrough,
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrACMEDomainNotResolvable):
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
