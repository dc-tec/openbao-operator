package configuration

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	portworkload "github.com/dc-tec/openbao-operator/internal/port/workload"
)

// PolicyClientFactory authenticates with the controller's JWT identity only.
type PolicyClientFactory func(context.Context, *openbaov1alpha1.OpenBaoCluster) (portopenbao.PolicyClient, error)

// PolicyManager restores approved built-in policy contents. It never writes
// the approval policy, JWT roles, or auth mount configuration.
type PolicyManager struct{ ClientFor PolicyClientFactory }

const policyVerificationInterval = 5 * time.Minute

// PolicyRevision identifies the desired bundle, including its exact contents.
func PolicyRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	// Approval permits both upgrade strategies, but verification must track the
	// contents currently required by the workload, including an active upgrade.
	hash := sha256.New()
	for _, policy := range configbuilder.OperatorPolicies(policyConfiguration(cluster)) {
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00", policy.Name, policy.Policy)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// policyConfiguration retains the strategy permissions needed by an unfinished
// upgrade. Administrator approval permits both supported strategies.
func policyConfiguration(cluster *openbaov1alpha1.OpenBaoCluster) *openbaov1alpha1.OpenBaoCluster {
	upgradeActive := cluster.Status.Upgrade != nil ||
		(cluster.Status.OperationLock != nil && cluster.Status.OperationLock.Operation == openbaov1alpha1.ClusterOperationUpgrade) ||
		(cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.Phase != "" && cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle)
	if !upgradeActive || portworkload.EffectiveStrategy(cluster) == portworkload.DesiredStrategy(cluster) {
		return cluster
	}
	active := cluster.DeepCopy()
	if active.Spec.Upgrade == nil {
		active.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{}
	}
	active.Spec.Upgrade.Strategy = portworkload.EffectiveStrategy(cluster)
	return active
}

func policyDigest(contents string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(contents)))
}

// RequirePolicyReady checks only the policy needed by a new operation.
// Status records progress; OpenBao ACLs remain the authorization boundary.
func RequirePolicyReady(cluster *openbaov1alpha1.OpenBaoCluster, name string) error {
	if !portauth.PolicyReconciliationEnabled(cluster) {
		return nil
	}
	for _, policy := range configbuilder.OperatorPolicies(policyConfiguration(cluster)) {
		if policy.Name != name {
			continue
		}
		if cluster.Status.Workload == nil || cluster.Status.Workload.PolicyReconciliation == nil {
			return nil // Enrollment has not established any observations yet.
		}
		observed, known := cluster.Status.Workload.PolicyReconciliation.Revisions[name]
		if !known || observed == policyDigest(policy.Policy) {
			return nil
		}
		return operatorerrors.WithReason(constants.ReasonPoliciesNotReady, operatorerrors.WrapTransientClusterState(
			fmt.Errorf("waiting for approved policy %s; inspect status.workload.policyReconciliation", name)))
	}
	return nil // The corresponding optional operation is not configured.
}

// Reconcile reads each policy and writes only missing or changed contents.
// One rejected policy does not prevent another policy from being checked or repaired.
func (m *PolicyManager) Reconcile(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	if !portauth.PolicyReconciliationEnabled(cluster) || !cluster.Status.Initialized {
		return recon.Result{}, nil
	}
	if cluster.Status.Workload == nil {
		cluster.Status.Workload = &openbaov1alpha1.WorkloadControllerStatus{}
	}
	if cluster.Status.Workload.PolicyReconciliation == nil {
		cluster.Status.Workload.PolicyReconciliation = &openbaov1alpha1.PolicyReconciliationStatus{}
	}
	status := cluster.Status.Workload.PolicyReconciliation
	revision := PolicyRevision(cluster)
	if status.AttemptedRevision == revision {
		if status.RetryAfter != nil && time.Now().Before(status.RetryAfter.Time) {
			return recon.Result{RequeueAfter: time.Until(status.RetryAfter.Time)}, nil
		}
		if status.LastError == nil && cluster.Status.Workload.PolicyRevision == revision && status.LastVerified != nil {
			if remaining := time.Until(status.LastVerified.Add(policyVerificationInterval)); remaining > 0 {
				return recon.Result{RequeueAfter: remaining}, nil
			}
		}
	}
	status.AttemptedRevision = revision
	status.RetryAfter = nil
	if status.Revisions == nil {
		status.Revisions = make(map[string]string)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if m == nil || m.ClientFor == nil {
		return recordPolicyFailure(status, fmt.Errorf("policy client factory is not configured"), 30*time.Second)
	}
	client, err := m.ClientFor(ctx, cluster)
	if err != nil {
		return recordPolicyFailure(status, err, policyRetryDelay(err))
	}
	var failures []error
	var delay time.Duration
	for _, policy := range configbuilder.OperatorPolicies(policyConfiguration(cluster)) {
		err := reconcilePolicy(ctx, client, policy, status.Revisions)
		if err != nil {
			failures = append(failures, fmt.Errorf("policy %s: %w", policy.Name, err))
			delay = max(delay, policyRetryDelay(err))
		}
	}
	if len(failures) != 0 {
		return recordPolicyFailure(status, errors.Join(failures...), delay)
	}
	status.LastError = nil
	verified := metav1.Now()
	status.LastVerified = &verified
	cluster.Status.Workload.PolicyRevision = revision
	return recon.Result{RequeueAfter: policyVerificationInterval}, nil
}

func reconcilePolicy(ctx context.Context, client portopenbao.PolicyClient, policy configbuilder.OperatorPolicy, revisions map[string]string) error {
	current, err := client.ReadACLPolicy(ctx, policy.Name)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if current == nil || *current != policy.Policy {
		revisions[policy.Name] = ""
		if err := client.WriteACLPolicy(ctx, policy.Name, policy.Policy); err != nil {
			return fmt.Errorf("write: %w", err)
		}
	}
	revisions[policy.Name] = policyDigest(policy.Policy)
	return nil
}

func policyRetryDelay(err error) time.Duration {
	if portopenbao.IsStatus(err, http.StatusForbidden) || portopenbao.IsStatus(err, http.StatusNotFound) {
		return 5 * time.Minute
	}
	return 30 * time.Second
}

func recordPolicyFailure(status *openbaov1alpha1.PolicyReconciliationStatus, err error, delay time.Duration) (recon.Result, error) {
	err = operatorerrors.WithReason(constants.ReasonPolicyReconciliationFailed, fmt.Errorf(
		"cannot reconcile operational policies; verify approval, JWT role, and auth method: %w", err))
	now, retry := metav1.Now(), metav1.NewTime(time.Now().Add(delay))
	status.LastError = &openbaov1alpha1.ControllerErrorStatus{Reason: constants.ReasonPolicyReconciliationFailed, Message: err.Error(), At: &now}
	status.RetryAfter = &retry
	return recon.Result{RequeueAfter: delay}, err
}
