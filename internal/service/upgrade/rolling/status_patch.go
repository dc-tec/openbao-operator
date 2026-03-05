package rolling

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
)

// ssaFieldOwner matches the AdminOps controller's SSA field owner.
//
// The rolling upgrade manager is invoked as a sub-reconciler from the AdminOps
// controller. It must not take ownership of unrelated status fields (for example,
// activeLeader or conditions) which are owned by other controllers.
const ssaFieldOwner = "openbao-adminops-controller"

// patchStatusSSA updates the cluster status using Server-Side Apply.
// SSA eliminates race conditions by having the API server merge changes,
// rather than requiring the client to refresh and merge manually.
func (m *Manager) patchStatusSSA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Apply only the rolling upgrade status we mutate. Other status fields are
	// owned and applied by their respective controllers.
	applyCluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: cluster.Status.Upgrade,
		},
	}

	applyConfig, err := kube.ToApplyConfiguration(applyCluster, m.client)
	if err != nil {
		return fmt.Errorf("failed to convert cluster to ApplyConfiguration: %w", err)
	}

	return m.client.Status().Apply(ctx, applyConfig,
		client.FieldOwner(ssaFieldOwner),
	)
}
