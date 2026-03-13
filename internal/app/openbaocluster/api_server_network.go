package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
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

// APIServerNetworkReasonPolicy configures low-cardinality reasons surfaced by
// the controller for Kubernetes API egress readiness.
type APIServerNetworkReasonPolicy struct {
	Ready                string
	Recommended          string
	ConfigurationInvalid string
}

// APIServerNetworkResult is the controller-facing evaluation result for the
// operator-managed Kubernetes API egress contract.
type APIServerNetworkResult struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

func (p APIServerNetworkReasonPolicy) readyReason() string {
	return fallbackReason(p.Ready, "APIServerNetworkReady")
}

func (p APIServerNetworkReasonPolicy) recommendedReason() string {
	return fallbackReason(p.Recommended, "APIServerEndpointIPsRecommended")
}

func (p APIServerNetworkReasonPolicy) configurationInvalidReason() string {
	return fallbackReason(p.ConfigurationInvalid, "APIServerNetworkConfigurationInvalid")
}

// EvaluateAPIServerNetwork evaluates the operator-known Kubernetes API egress
// contract for operator-managed NetworkPolicies.
func EvaluateAPIServerNetwork(
	ctx context.Context,
	deps APIServerNetworkDependencies,
	reasons APIServerNetworkReasonPolicy,
	cluster *openbaov1alpha1.OpenBaoCluster,
) APIServerNetworkResult {
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

	readiness := manager.EvaluateAPIServerNetworkReadiness(ctx, logr.Discard(), cluster)
	switch readiness.Status {
	case metav1.ConditionTrue:
		readiness.Reason = reasons.readyReason()
	case metav1.ConditionUnknown:
		readiness.Reason = reasons.recommendedReason()
	default:
		readiness.Reason = reasons.configurationInvalidReason()
	}

	return APIServerNetworkResult{
		Status:  readiness.Status,
		Reason:  readiness.Reason,
		Message: readiness.Message,
	}
}
