package infra

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// APIServerNetworkReadiness reports how strongly the operator can validate the
// Kubernetes API egress contract used by operator-managed NetworkPolicies.
type APIServerNetworkReadiness struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

// EvaluateAPIServerNetworkReadiness validates the operator-known Kubernetes API
// egress contract for main and job NetworkPolicies. Unknown means the common
// service-VIP path is configured, but environment-specific post-DNAT endpoint
// requirements cannot be proven without explicit endpoint IPs.
func (m *Manager) EvaluateAPIServerNetworkReadiness(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) APIServerNetworkReadiness {
	info, err := m.detectAPIServerInfo(ctx, logger, cluster)
	if err != nil {
		wrapped := wrapAPIServerNetworkConfigurationError("primary", err)
		return APIServerNetworkReadiness{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonAPIServerNetworkConfigurationInvalid,
			Message: wrapped.Error(),
		}
	}
	if info == nil || (info.ServiceNetworkCIDR == "" && len(info.EndpointIPs) == 0) {
		wrapped := wrapAPIServerNetworkConfigurationError("primary", nil)
		return APIServerNetworkReadiness{
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonAPIServerNetworkConfigurationInvalid,
			Message: wrapped.Error(),
		}
	}

	if len(info.EndpointIPs) == 0 {
		return APIServerNetworkReadiness{
			Status: metav1.ConditionUnknown,
			Reason: constants.ReasonAPIServerEndpointIPsRecommended,
			Message: fmt.Sprintf(
				"Kubernetes API egress is configured through the service VIP (%s). This is sufficient on many clusters. If your CNI enforces egress on post-DNAT traffic, also configure spec.network.apiServerEndpointIPs with the control-plane endpoint IPs.",
				info.ServiceNetworkCIDR,
			),
		}
	}

	return APIServerNetworkReadiness{
		Status: metav1.ConditionTrue,
		Reason: constants.ReasonAPIServerNetworkReady,
		Message: fmt.Sprintf(
			"Kubernetes API egress is configured with service VIP %s and explicit endpoint IPs %v.",
			info.ServiceNetworkCIDR,
			info.EndpointIPs,
		),
	}
}
