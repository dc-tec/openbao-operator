package infra

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// Manager handles deletion cleanup for an OpenBaoCluster.
type Manager struct {
	client client.Client
}

func NewManager(c client.Client) *Manager {
	return &Manager{
		client: c,
	}
}

// Cleanup handles resources that require special deletion logic beyond Kubernetes Garbage Collection.
//
// Most infrastructure resources (StatefulSet, Services, ConfigMaps, RBAC, etc.) have OwnerReferences
// set to the OpenBaoCluster and are automatically deleted by Kubernetes GC when the cluster is deleted.
// This method only handles resources that need explicit policy-based handling:
//   - PVCs: Only deleted when DeletionPolicy is DeletePVCs or DeleteAll
//
// Note: Secret preservation for DeletionPolicy=Retain is handled by the deletion controller
// (deletion.go orphanSecretsForRetention) which removes OwnerReferences before finalization.
//
// It is safe to call Cleanup multiple times; missing resources are treated as successfully deleted.
func (m *Manager) Cleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, policy openbaov1alpha1.DeletionPolicy) error {
	if policy == "" {
		policy = openbaov1alpha1.DeletionPolicyRetain
	}

	logger = logger.WithValues("deletionPolicy", string(policy))
	logger.Info("Processing cleanup for deleted OpenBaoCluster",
		"note", "Most resources are deleted by Kubernetes GC via OwnerReferences")

	// PVCs require explicit deletion based on policy because they are not owned by the
	// StatefulSet (they use volumeClaimTemplates which creates independent PVCs).
	// Kubernetes GC does not automatically delete these when the OpenBaoCluster is deleted.
	if policy == openbaov1alpha1.DeletionPolicyDeletePVCs || policy == openbaov1alpha1.DeletionPolicyDeleteAll {
		if err := m.deletePVCs(ctx, cluster); err != nil {
			return fmt.Errorf("failed to delete PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}
		logger.Info("PVCs deleted per deletion policy")
	} else {
		logger.Info("Preserving PVCs per Retain policy")
	}

	return nil
}
