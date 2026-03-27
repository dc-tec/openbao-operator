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

// ACMEIntegrationReasonPolicy configures low-cardinality reasons surfaced by
// the controller for ACME integration readiness.
type ACMEIntegrationReasonPolicy struct {
	Ready                               string
	Unknown                             string
	GatewayAPIMissing                   string
	PrerequisitesMissing                string
	ACMEDomainNotResolvable             string
	ACMEGatewayNotConfiguredPassthrough string
}

// ACMEIntegrationResult is the controller-facing evaluation result for the
// operator-managed ACME integration contract.
type ACMEIntegrationResult struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

func (p ACMEIntegrationReasonPolicy) readyReason() string {
	return fallbackReason(p.Ready, constants.ReasonACMEIntegrationReady)
}

func (p ACMEIntegrationReasonPolicy) unknownReason() string {
	return fallbackReason(p.Unknown, constants.ReasonUnknown)
}

func (p ACMEIntegrationReasonPolicy) gatewayAPIMissingReason() string {
	return fallbackReason(p.GatewayAPIMissing, constants.ReasonGatewayAPIMissing)
}

func (p ACMEIntegrationReasonPolicy) prerequisitesMissingReason() string {
	return fallbackReason(p.PrerequisitesMissing, constants.ReasonPrerequisitesMissing)
}

func (p ACMEIntegrationReasonPolicy) acmeDomainNotResolvableReason() string {
	return fallbackReason(p.ACMEDomainNotResolvable, constants.ReasonACMEDomainNotResolvable)
}

func (p ACMEIntegrationReasonPolicy) acmeGatewayNotConfiguredReason() string {
	return fallbackReason(p.ACMEGatewayNotConfiguredPassthrough, constants.ReasonACMEGatewayNotConfiguredForPassthrough)
}

// EvaluateACMEIntegration validates the operator-managed prerequisites around
// OpenBao's native ACME flow and returns controller-ready status information.
func EvaluateACMEIntegration(
	ctx context.Context,
	deps ACMEIntegrationDependencies,
	reasons ACMEIntegrationReasonPolicy,
	cluster *openbaov1alpha1.OpenBaoCluster,
) ACMEIntegrationResult {
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
	err := manager.ValidateACMEPreflight(ctx, logr.Discard(), cluster)

	switch {
	case err == nil:
		return ACMEIntegrationResult{
			Status:  metav1.ConditionTrue,
			Reason:  reasons.readyReason(),
			Message: "ACME integration prerequisites are satisfied",
		}
	case errors.Is(err, inframanager.ErrGatewayAPIMissing):
		return ACMEIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.gatewayAPIMissingReason(),
			Message: "Gateway API CRDs required for ACME passthrough are not installed",
		}
	case errors.Is(err, inframanager.ErrACMEGatewayNotConfiguredForPassthrough):
		return ACMEIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.acmeGatewayNotConfiguredReason(),
			Message: err.Error(),
		}
	case errors.Is(err, inframanager.ErrACMEDomainNotResolvable):
		return ACMEIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.acmeDomainNotResolvableReason(),
			Message: err.Error(),
		}
	case operatorerrors.IsPermanent(err):
		return ACMEIntegrationResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasons.prerequisitesMissingReason(),
			Message: err.Error(),
		}
	default:
		return ACMEIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasons.unknownReason(),
			Message: fmt.Sprintf("Failed to evaluate ACME integration prerequisites: %v", err),
		}
	}
}

func fallbackReason(got, fallback string) string {
	if got != "" {
		return got
	}
	return fallback
}
