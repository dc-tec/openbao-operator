package upgrade

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/kube"
)

// EnsureUpgradeServiceAccount creates or updates the ServiceAccount for upgrade executor Jobs using
// Server-Side Apply. This ServiceAccount is used by upgrade executor Jobs for JWT Auth to OpenBao.
func EnsureUpgradeServiceAccount(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, fieldOwner string) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	if fieldOwner == "" {
		fieldOwner = "openbao-operator"
	}

	saName := cluster.Name + constants.SuffixUpgradeServiceAccount

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:      cluster.Name,
				constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:   cluster.Name,
				constants.LabelOpenBaoComponent: "upgrade",
			},
		},
	}

	applyConfig, err := kube.ToApplyConfiguration(sa, c)
	if err != nil {
		return fmt.Errorf("failed to convert ServiceAccount to ApplyConfiguration: %w", err)
	}

	applyOpts := []client.ApplyOption{
		client.ForceOwnership,
		client.FieldOwner(fieldOwner),
	}

	if err := c.Apply(ctx, applyConfig, applyOpts...); err != nil {
		return fmt.Errorf("failed to ensure upgrade ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
	}

	return nil
}
