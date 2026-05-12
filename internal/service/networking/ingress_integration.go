package networking

import (
	"context"
	"fmt"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// ValidateIngressIntegration evaluates the operator-known ingress contract for
// the selected ingress mode. It validates the referenced IngressClass when one
// is configured and evaluates the managed Ingress readiness posture.
func (m *Manager) ValidateIngressIntegration(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Ingress == nil || !cluster.Spec.Ingress.Enabled {
		return nil
	}

	className := ""
	if cluster.Spec.Ingress.ClassName != nil {
		className = strings.TrimSpace(*cluster.Spec.Ingress.ClassName)
	}
	if className != "" {
		ingressClass := &networkingv1.IngressClass{}
		if err := m.reader.Get(ctx, types.NamespacedName{Name: className}, ingressClass); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("%w: referenced IngressClass %q was not found", ErrIngressClassMissing, className)
			}
			if apierrors.IsForbidden(err) {
				return fmt.Errorf(
					"%w: cannot verify referenced IngressClass %q because the operator cannot read it: %v",
					ErrIngressCapabilitiesUnknown,
					className,
					err,
				)
			}
			return fmt.Errorf("failed to get referenced IngressClass %q: %w", className, err)
		}
	}

	ingress := &networkingv1.Ingress{}
	key := types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name}
	if err := m.reader.Get(ctx, key, ingress); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: managed Ingress %s/%s was not found", ErrIngressObjectMissing, key.Namespace, key.Name)
		}
		if apierrors.IsForbidden(err) {
			return fmt.Errorf(
				"%w: cannot verify managed Ingress %s/%s because the operator cannot read it: %v",
				ErrIngressCapabilitiesUnknown,
				key.Namespace,
				key.Name,
				err,
			)
		}
		return fmt.Errorf("failed to get managed Ingress %s/%s: %w", key.Namespace, key.Name, err)
	}

	if ingressReadinessMode(cluster.Spec.Ingress) == openbaov1alpha1.IngressReadinessModeCreated {
		return nil
	}
	if len(ingress.Status.LoadBalancer.Ingress) == 0 {
		return fmt.Errorf("%w: managed Ingress %s/%s has not published a load balancer address yet", ErrIngressLoadBalancerPending, ingress.Namespace, ingress.Name)
	}

	return nil
}

func ingressReadinessMode(ingress *openbaov1alpha1.IngressConfig) openbaov1alpha1.IngressReadinessMode {
	if ingress == nil || ingress.ReadinessMode == "" {
		return openbaov1alpha1.IngressReadinessModeLoadBalancerPublished
	}
	return ingress.ReadinessMode
}
