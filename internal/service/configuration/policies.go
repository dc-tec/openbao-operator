package configuration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// PolicyClientFactory authenticates with the controller's JWT identity only.
type PolicyClientFactory func(context.Context, *openbaov1alpha1.OpenBaoCluster) (portopenbao.PolicyWriter, error)

// PolicyManager restores approved built-in policy contents. It never writes
// the approval policy, JWT roles, or auth mount configuration.
type PolicyManager struct{ ClientFor PolicyClientFactory }

// PolicyRevision identifies the desired bundle, including its exact contents.
func PolicyRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(configbuilder.OperatorPolicyApproval(cluster))))
}

// RequirePoliciesReady prevents new operations from using a partial or outdated
// bundle. OpenBao ACLs provide authorization; status only records progress.
func RequirePoliciesReady(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if !portauth.PolicyReconciliationEnabled(cluster) {
		return nil
	}
	if cluster.Status.Workload == nil || cluster.Status.Workload.PolicyRevision != PolicyRevision(cluster) {
		return operatorerrors.WithReason("PoliciesNotReady", operatorerrors.WrapTransientClusterState(
			fmt.Errorf("waiting for approved operational policies; inspect status.workload.lastError and the OpenBao policy approval")))
	}
	return nil
}

// Reconcile writes the entire approved bundle before publishing its revision.
// Repeated writes avoid granting policy-read permissions or relying on a cache
// to detect deletions made outside Kubernetes.
func (m *PolicyManager) Reconcile(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	if !portauth.PolicyReconciliationEnabled(cluster) || !cluster.Status.Initialized {
		return recon.Result{}, nil
	}
	if cluster.Status.Workload == nil {
		cluster.Status.Workload = &openbaov1alpha1.WorkloadControllerStatus{}
	}
	cluster.Status.Workload.PolicyRevision = ""
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if m == nil || m.ClientFor == nil {
		return recon.Result{}, policyReconciliationError(fmt.Errorf("policy client factory is not configured"))
	}
	writer, err := m.ClientFor(ctx, cluster)
	if err != nil {
		return recon.Result{}, policyReconciliationError(err)
	}
	for _, policy := range configbuilder.OperatorPolicies(cluster) {
		if err := writer.WriteACLPolicy(ctx, policy.Name, policy.Policy); err != nil {
			return recon.Result{}, policyReconciliationError(fmt.Errorf("policy %s: %w", policy.Name, err))
		}
	}
	cluster.Status.Workload.PolicyRevision = PolicyRevision(cluster)
	return recon.Result{}, nil
}

func policyReconciliationError(err error) error {
	return operatorerrors.WithReason("PolicyReconciliationFailed", operatorerrors.WrapTransientClusterState(
		fmt.Errorf("cannot reconcile operational policies; an OpenBao administrator must verify approval, JWT role, and auth method: %w", err)))
}
