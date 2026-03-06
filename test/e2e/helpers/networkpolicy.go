package helpers

import (
	"context"
	"fmt"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AllowEgressToOperatorNamespace patches the NetworkPolicy for the given cluster
// to allow egress to the operator namespace. This is needed for e2e tests where
// transit seal backends (infra-bao) run in the operator namespace.
func AllowEgressToOperatorNamespace(
	ctx context.Context,
	c client.Client,
	clusterName string,
	clusterNamespace string,
	operatorNamespace string,
) error {
	networkPolicyName := clusterName + "-network-policy"
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		networkPolicy := &networkingv1.NetworkPolicy{}
		if err := c.Get(ctx, types.NamespacedName{
			Name:      networkPolicyName,
			Namespace: clusterNamespace,
		}, networkPolicy); err != nil {
			return fmt.Errorf("failed to get NetworkPolicy %s/%s: %w", clusterNamespace, networkPolicyName, err)
		}

		// Check if rule already exists to avoid duplication in retry loop
		for _, egress := range networkPolicy.Spec.Egress {
			for _, to := range egress.To {
				if to.NamespaceSelector != nil &&
					to.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == operatorNamespace {
					return nil // Already exists
				}
			}
		}

		// Create egress rule for operator namespace
		operatorNamespacePeer := networkingv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"kubernetes.io/metadata.name": operatorNamespace,
				},
			},
		}

		apiPort := intstr.FromInt(constants.PortAPI)
		egressRule := networkingv1.NetworkPolicyEgressRule{
			// Allow egress to operator namespace on OpenBao API port (for transit seal in e2e tests)
			To: []networkingv1.NetworkPolicyPeer{operatorNamespacePeer},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &apiPort,
				},
			},
		}

		// Append the egress rule to existing egress rules
		networkPolicy.Spec.Egress = append(networkPolicy.Spec.Egress, egressRule)

		if err := c.Update(ctx, networkPolicy); err != nil {
			return fmt.Errorf("failed to update NetworkPolicy %s/%s: %w", clusterNamespace, networkPolicyName, err)
		}
		return nil
	})
}
