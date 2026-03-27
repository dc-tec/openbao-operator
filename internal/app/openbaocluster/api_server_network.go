package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
)

// APIServerNetworkDependencies groups dependencies required to evaluate the
// operator-managed Kubernetes API egress contract.
type APIServerNetworkDependencies struct {
	Client            client.Client
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	OperatorNamespace string
	Platform          string
}

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
	deps APIServerNetworkDependencies,
	cluster *openbaov1alpha1.OpenBaoCluster,
) APIServerNetworkResult {
	manager := inframanager.NewManagerWithReaderAndOIDCConfig(
		deps.Client,
		deps.APIReader,
		deps.Scheme,
		deps.OperatorNamespace,
		nil,
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
