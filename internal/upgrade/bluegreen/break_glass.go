package bluegreen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
)

func (m *Manager) shouldHaltForBreakGlass(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster.Status.BreakGlass == nil || !cluster.Status.BreakGlass.Active {
		return false
	}
	if cluster.Spec.BreakGlassAck == cluster.Status.BreakGlass.Nonce {
		return false
	}
	logger.Info("Cluster is in break glass mode; halting blue/green reconciliation",
		"breakGlassReason", cluster.Status.BreakGlass.Reason,
		"breakGlassNonce", cluster.Status.BreakGlass.Nonce)
	return true
}

func (m *Manager) handleBreakGlassAck(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (handled bool, result recon.Result) {
	if cluster.Status.BreakGlass == nil || !cluster.Status.BreakGlass.Active {
		return false, recon.Result{}
	}

	if cluster.Status.BreakGlass.Nonce == "" || cluster.Spec.BreakGlassAck != cluster.Status.BreakGlass.Nonce {
		return true, recon.Result{}
	}

	now := metav1.Now()
	cluster.Status.BreakGlass.Active = false
	cluster.Status.BreakGlass.AcknowledgedAt = &now

	logger.Info("Break glass acknowledged; resuming automation", "reason", cluster.Status.BreakGlass.Reason)

	if cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseRollingBack &&
		cluster.Status.BreakGlass.Reason == openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed {
		cluster.Status.BlueGreen.RollbackAttempt++
		cluster.Status.BlueGreen.JobFailureCount = 0
		cluster.Status.BlueGreen.LastJobFailure = ""
		return true, requeueShort()
	}

	return true, requeueShort()
}

const breakGlassNonceBytes = 16

func rollbackRunID(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.RollbackAttempt <= 0 {
		return "rollback"
	}
	return fmt.Sprintf("rollback-retry-%d", cluster.Status.BlueGreen.RollbackAttempt)
}

func (m *Manager) enterBreakGlassRollbackConsensusRepairFailed(cluster *openbaov1alpha1.OpenBaoCluster, jobName string) {
	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
		return
	}

	now := metav1.Now()

	nonce := newBreakGlassNonce()
	message := fmt.Sprintf("Rollback consensus repair Job %s failed; manual intervention required", jobName)

	steps := []string{
		fmt.Sprintf("Inspect rollback Job logs: kubectl -n %s logs job/%s", cluster.Namespace, jobName),
		fmt.Sprintf("Inspect pod status: kubectl -n %s get pods -l %s=%s -o wide", cluster.Namespace, constants.LabelOpenBaoCluster, cluster.Name),
		fmt.Sprintf("Attempt to identify Raft leader: kubectl -n %s exec -it %s-0 -- bao operator raft list-peers", cluster.Namespace, cluster.Name),
		"Perform any required Raft recovery steps (peer removal, snapshot restore) per the user-guide runbook.",
		fmt.Sprintf("Acknowledge break glass to retry rollback automation: kubectl -n %s patch openbaocluster %s --type merge -p '{\"spec\":{\"breakGlassAck\":\"%s\"}}'", cluster.Namespace, cluster.Name, nonce),
	}

	cluster.Status.BreakGlass = &openbaov1alpha1.BreakGlassStatus{
		Active:    true,
		Reason:    openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
		Message:   message,
		Nonce:     nonce,
		EnteredAt: &now,
		Steps:     steps,
	}
}

func newBreakGlassNonce() string {
	buf := make([]byte, breakGlassNonceBytes)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}
