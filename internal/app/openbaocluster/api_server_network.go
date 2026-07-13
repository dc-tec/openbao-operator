package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	networkingmanager "github.com/dc-tec/openbao-operator/internal/service/networking"
)

// APIServerNetworkResult is the controller-facing evaluation result for the
// operator-managed Kubernetes API egress contract.
type APIServerNetworkResult struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

// EvaluateAPIServerNetwork evaluates the operator-known Kubernetes API egress
// contract for operator-managed NetworkPolicies.
func EvaluateAPIServerNetwork(
	ctx context.Context,
	deps StatusIntegrationDependencies,
	cluster *openbaov1alpha1.OpenBaoCluster,
) APIServerNetworkResult {
	manager := networkingmanager.NewManagerWithReader(
		deps.Client,
		deps.APIReader,
		deps.Scheme,
		deps.OperatorNamespace,
		deps.Platform,
	)

	readiness := manager.EvaluateAPIServerNetworkReadiness(ctx, logr.Discard(), cluster)
	switch readiness.Status {
	case metav1.ConditionTrue:
		readiness.Reason = constants.ReasonAPIServerNetworkReady
	case metav1.ConditionUnknown:
		readiness.Reason = constants.ReasonAPIServerEndpointIPsRecommended
	default:
		readiness.Reason = constants.ReasonAPIServerNetworkConfigurationInvalid
	}

	return APIServerNetworkResult{
		Status:  readiness.Status,
		Reason:  readiness.Reason,
		Message: readiness.Message,
	}
}
