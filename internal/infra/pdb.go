package infra

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

// ensurePodDisruptionBudget creates or updates a PodDisruptionBudget for the OpenBaoCluster.
// This prevents Kubernetes from evicting too many pods simultaneously during node drains
// or cluster upgrades, protecting Raft quorum.
//
// SECURITY: Without a PDB, voluntary disruptions (node drains, cluster autoscaler)
// could evict all pods simultaneously, causing quorum loss and potential data loss.
func (m *Manager) ensurePodDisruptionBudget(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	pdbName := pdbName(cluster)

	// For single-replica clusters, PDB doesn't make sense
	if cluster.Spec.Replicas < 2 {
		logger.V(1).Info("Skipping PDB creation for single-replica cluster", "replicas", cluster.Spec.Replicas)
		return nil
	}

	// maxUnavailable = 1 ensures at least N-1 pods remain available during disruptions.
	// For a 3-replica cluster, this means 2 pods must remain available (Raft quorum).
	// For a 5-replica cluster, 4 pods remain available (well above Raft quorum of 3).
	maxUnavailable := intstr.FromInt(1)

	// SECURITY: PDB selector must exclude backup/restore/upgrade Job pods.
	// Jobs don't support the scale subresource that PDBs require for calculating
	// expected pod counts, causing errors like "jobs.batch does not implement
	// the scale subresource".
	pdb := &policyv1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy/v1",
			Kind:       "PodDisruptionBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: podSelectorLabels(cluster),
				// Exclude Job pods (backup, restore, upgrade-snapshot) which don't
				// support the scale subresource required by PDBs.
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      constants.LabelOpenBaoComponent,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"backup", "restore", "upgrade", "upgrade-snapshot", "validation-hook", "post-switch-validation-hook"},
					},
				},
			},
		},
	}

	if err := m.applyResource(ctx, pdb, cluster); err != nil {
		return fmt.Errorf("failed to apply PodDisruptionBudget %s/%s: %w", cluster.Namespace, pdbName, err)
	}

	logger.V(1).Info("PodDisruptionBudget reconciled", "name", pdbName)
	return nil
}

// pdbName returns the name of the PodDisruptionBudget for the cluster.
func pdbName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + "-pdb"
}
