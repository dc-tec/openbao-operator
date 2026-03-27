package openbaocluster

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

const (
	upgradeRequestRetryFieldPath   = "spec.upgrade.requests.retry"
	upgradeRequestPromoteFieldPath = "spec.upgrade.requests.promote"
	operatorJWTAuthMount           = "jwt-operator"
)

func evaluateProductionReady(cluster *openbaov1alpha1.OpenBaoCluster, admissionReady bool, admissionSummary string) (metav1.ConditionStatus, string, string) {
	if cluster.Spec.Profile == "" {
		return metav1.ConditionFalse, ReasonProfileNotSet, "spec.profile must be explicitly set to Hardened or Development"
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		return metav1.ConditionFalse, ReasonDevelopmentProfile, "Development profile is not suitable for production"
	}

	if !admissionReady {
		if admissionSummary != "" {
			return metav1.ConditionFalse, ReasonAdmissionPoliciesNotReady, "Required admission policies are not ready: " + admissionSummary
		}
		return metav1.ConditionFalse, ReasonAdmissionPoliciesNotReady, "Required admission policies are not ready"
	}

	if status, reason, message, blocked := requireConditionFalseOnly(
		cluster,
		openbaov1alpha1.ConditionAPIServerNetworkReady,
		"Kubernetes API egress prerequisites are not ready",
	); blocked {
		return status, reason, message
	}

	if cluster.Spec.TLS.Mode == "" || cluster.Spec.TLS.Mode == openbaov1alpha1.TLSModeOperatorManaged {
		return metav1.ConditionFalse, ReasonOperatorManagedTLS, "Hardened profile requires TLS mode External or ACME; OperatorManaged TLS is not considered production-ready"
	}

	if isStaticUnseal(cluster) {
		return metav1.ConditionFalse, ReasonStaticUnsealInUse, "Hardened profile requires a non-static unseal configuration (external KMS/Transit); static unseal is not considered production-ready"
	}

	if unsealTLSSkipVerifyEnabled(cluster) {
		return metav1.ConditionFalse, ReasonUnsealTLSSkipVerify, "Hardened profile requires TLS verification for external unseal backends; tlsSkipVerify is not considered production-ready"
	}

	if transitInlineTokenConfigured(cluster) {
		return metav1.ConditionFalse, ReasonTransitInlineToken, "Hardened profile does not allow spec.unseal.transit.token; use spec.unseal.credentialsSecretRef instead"
	}

	if transitAddressRequiresHTTPS(cluster) {
		return metav1.ConditionFalse, ReasonTransitAddressNotHTTPS, "Hardened profile requires spec.unseal.transit.address to use a valid HTTPS URL"
	}

	if usesCloudKMSUnseal(cluster) {
		if status, reason, message, blocked := requireConditionTrue(
			cluster,
			openbaov1alpha1.ConditionCloudUnsealIdentityReady,
			"Cloud KMS unseal identity prerequisites are not ready",
		); blocked {
			return status, reason, message
		}
	}

	if portopenbao.UsesACMEMode(cluster) {
		if status, reason, message, blocked := requireConditionTrue(
			cluster,
			openbaov1alpha1.ConditionACMEIntegrationReady,
			"ACME integration prerequisites are not ready",
		); blocked {
			return status, reason, message
		}
	}

	if portopenbao.RequiresSharedACMECache(cluster) {
		if status, reason, message, blocked := requireConditionTrue(
			cluster,
			openbaov1alpha1.ConditionACMECacheReady,
			"ACME shared cache is not ready for this topology",
		); blocked {
			return status, reason, message
		}
	}

	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		if status, reason, message, blocked := requireConditionNotFalse(
			cluster,
			openbaov1alpha1.ConditionGatewayIntegrationReady,
			"Gateway integration readiness has not been evaluated",
			"Gateway integration prerequisites are not ready",
		); blocked {
			return status, reason, message
		}
	}

	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		return metav1.ConditionFalse, ReasonRootTokenStored, "Hardened profile requires self-init; manual bootstrap stores a root token Secret and is not considered production-ready"
	}

	return metav1.ConditionTrue, ReasonProductionReady, "Cluster meets Hardened profile production-ready requirements"
}

func requireConditionTrue(
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	defaultMessage string,
) (metav1.ConditionStatus, string, string, bool) {
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
	if condition == nil || condition.Status != metav1.ConditionTrue {
		if condition != nil && condition.Reason != "" {
			return metav1.ConditionFalse, condition.Reason, condition.Message, true
		}
		return metav1.ConditionFalse, ReasonProductionNotReady, defaultMessage, true
	}
	return "", "", "", false
}

func requireConditionNotFalse(
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	missingMessage string,
	notReadyMessage string,
) (metav1.ConditionStatus, string, string, bool) {
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
	if condition == nil {
		return metav1.ConditionFalse, ReasonProductionNotReady, missingMessage, true
	}
	if condition.Status == metav1.ConditionFalse {
		if condition.Reason != "" {
			return metav1.ConditionFalse, condition.Reason, condition.Message, true
		}
		return metav1.ConditionFalse, ReasonProductionNotReady, notReadyMessage, true
	}
	return "", "", "", false
}

func requireConditionFalseOnly(
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	notReadyMessage string,
) (metav1.ConditionStatus, string, string, bool) {
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
	if condition == nil || condition.Status != metav1.ConditionFalse {
		return "", "", "", false
	}
	if condition.Reason != "" {
		return metav1.ConditionFalse, condition.Reason, condition.Message, true
	}
	return metav1.ConditionFalse, ReasonProductionNotReady, notReadyMessage, true
}

func isStaticUnseal(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster.Spec.Unseal == nil {
		return true
	}
	if cluster.Spec.Unseal.Type == "" {
		return true
	}
	return cluster.Spec.Unseal.Type == unsealTypeStatic
}

func unsealTLSSkipVerifyEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.Unseal == nil {
		return false
	}
	if cluster.Spec.Unseal.Transit != nil && cluster.Spec.Unseal.Transit.TLSSkipVerify != nil && *cluster.Spec.Unseal.Transit.TLSSkipVerify {
		return true
	}
	return false
}

func transitInlineTokenConfigured(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Spec.Unseal != nil &&
		cluster.Spec.Unseal.Transit != nil &&
		strings.TrimSpace(cluster.Spec.Unseal.Transit.Token) != ""
}

func transitAddressRequiresHTTPS(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.Unseal == nil || cluster.Spec.Unseal.Transit == nil {
		return false
	}

	address := strings.TrimSpace(cluster.Spec.Unseal.Transit.Address)
	if address == "" {
		return true
	}

	u, err := url.Parse(address)
	if err != nil {
		return true
	}

	return !strings.EqualFold(u.Scheme, "https") || strings.TrimSpace(u.Host) == ""
}

func usesCloudKMSUnseal(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.Unseal == nil {
		return false
	}

	switch cluster.Spec.Unseal.Type {
	case "awskms", "gcpckms", "azurekeyvault", "ocikms":
		return true
	default:
		return false
	}
}

// buildAvailableCondition builds the Available condition based on replica counts.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildAvailableCondition(cluster *openbaov1alpha1.OpenBaoCluster, readyReplicas int32) metav1.Condition {
	available := readyReplicas == cluster.Spec.Replicas && readyReplicas > 0

	if available {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionAvailable),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonAllReplicasReady,
			Message: fmt.Sprintf("All %d replicas are ready", readyReplicas),
		}
	}

	if readyReplicas == 0 {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionAvailable),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNoReplicasReady,
			Message: "No replicas are ready yet",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionAvailable),
		Status:  metav1.ConditionFalse,
		Reason:  ReasonNotReady,
		Message: fmt.Sprintf("Only %d/%d replicas are ready", readyReplicas, cluster.Spec.Replicas),
	}
}

// buildDegradedCondition builds the Degraded condition based on cluster state.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildDegradedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	upgradeFailed bool,
) metav1.Condition {
	// Check break glass first
	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  reasonBreakGlassRequired,
			Message: buildBreakGlassConditionMessage(cluster),
		}
	}

	// Check upgrade failure
	if upgradeFailed && cluster.Status.Upgrade != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  cluster.Status.Upgrade.LastErrorReason,
			Message: buildRollingUpgradeFailedMessage(cluster),
		}
	}

	// Check workload error
	if cluster.Status.Workload != nil && cluster.Status.Workload.LastError != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  cluster.Status.Workload.LastError.Reason,
			Message: cluster.Status.Workload.LastError.Message,
		}
	}

	// Check admin ops error
	if cluster.Status.AdminOps != nil && cluster.Status.AdminOps.LastError != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  cluster.Status.AdminOps.LastError.Reason,
			Message: cluster.Status.AdminOps.LastError.Message,
		}
	}

	// Check self-init disabled warning
	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionDegraded),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonRootTokenStored,
			Message: "SelfInit is disabled. The operator is storing the root token in a Secret, which violates Zero Trust principles. Anyone with Secret read access in this namespace can access the root token. Strongly consider enabling SelfInit (spec.selfInit.enabled=true) for production deployments.",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionDegraded),
		Status:  metav1.ConditionFalse,
		Reason:  reasonReconciling,
		Message: "No degradation has been recorded by the controller",
	}
}

// buildUserAccessBootstrapCondition reports whether the operator can recognize
// a likely user-facing authentication bootstrap path in self-init requests.
// This is intentionally best-effort and never blocks reconciliation.
func buildUserAccessBootstrapCondition(cluster *openbaov1alpha1.OpenBaoCluster) metav1.Condition {
	selfInitEnabled := cluster != nil && cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUserAccessBootstrap),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonDisabled,
			Message: "Self-init is disabled; user access bootstrap heuristics are not evaluated",
		}
	}

	mounts := recognizedUserAccessBootstrapMounts(cluster.Spec.SelfInit.Requests)
	if len(mounts) == 0 {
		return metav1.Condition{
			Type:   string(openbaov1alpha1.ConditionUserAccessBootstrap),
			Status: metav1.ConditionUnknown,
			Reason: ReasonUserAccessUnverified,
			Message: "Self-init is enabled, but the operator could not verify a user-facing authentication bootstrap path from spec.selfInit.requests. " +
				"spec.selfInit.oidc only bootstraps operator authentication. Verify that self-init requests create a user-facing auth method plus the roles, mappings, or accounts your operators need before relying on self-init.",
		}
	}

	return metav1.Condition{
		Type:   string(openbaov1alpha1.ConditionUserAccessBootstrap),
		Status: metav1.ConditionTrue,
		Reason: ReasonUserAccessConfigured,
		Message: fmt.Sprintf(
			"The operator recognized self-init requests that appear to bootstrap user authentication on %s. This is a best-effort heuristic; verify the resulting roles, mappings, and credentials before relying on self-init.",
			strings.Join(mounts, ", "),
		),
	}
}

func recognizedUserAccessBootstrapMounts(requests []openbaov1alpha1.SelfInitRequest) []string {
	if len(requests) == 0 {
		return nil
	}

	mounts := make(map[string]struct{}, len(requests))
	for _, req := range requests {
		mount, ok := recognizedUserAccessBootstrapMount(req)
		if !ok {
			continue
		}
		mounts[mount] = struct{}{}
	}

	if len(mounts) == 0 {
		return nil
	}

	out := make([]string, 0, len(mounts))
	for mount := range mounts {
		out = append(out, mount)
	}
	sort.Strings(out)
	return out
}

func recognizedUserAccessBootstrapMount(req openbaov1alpha1.SelfInitRequest) (string, bool) {
	path := strings.Trim(strings.ToLower(req.Path), "/")
	if path == "" {
		return "", false
	}

	if strings.HasPrefix(path, "auth/") {
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			return "", false
		}
		mount := parts[1]
		if mount == "" || mount == operatorJWTAuthMount || mount == "token" {
			return "", false
		}
		return "auth/" + mount, true
	}

	if strings.HasPrefix(path, "sys/auth/") {
		parts := strings.Split(path, "/")
		if len(parts) < 3 {
			return "", false
		}
		mount := parts[2]
		if mount == "" || mount == operatorJWTAuthMount || req.AuthMethod == nil {
			return "", false
		}
		if !isLikelyUserAuthMethod(req.AuthMethod.Type) {
			return "", false
		}
		return "auth/" + mount, true
	}

	return "", false
}

func isLikelyUserAuthMethod(methodType string) bool {
	switch strings.TrimSpace(strings.ToLower(methodType)) {
	case "jwt", "kubernetes", "userpass", "ldap", "oidc", "approle", "cert":
		return true
	default:
		return false
	}
}

// buildUpgradingCondition builds the Upgrading condition based on upgrade state.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildUpgradingCondition(cluster *openbaov1alpha1.OpenBaoCluster) metav1.Condition {
	rollingUpgradeInProgress := cluster.Status.Upgrade != nil
	upgradeFailed := rollingUpgradeInProgress && cluster.Status.Upgrade.LastErrorReason != ""

	blueGreenInProgress := cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.Phase != "" &&
		cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle

	if upgradeFailed && cluster.Status.Upgrade != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUpgrading),
			Status:  metav1.ConditionFalse,
			Reason:  cluster.Status.Upgrade.LastErrorReason,
			Message: buildRollingUpgradeFailedMessage(cluster),
		}
	}

	if rollingUpgradeInProgress && !upgradeFailed {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUpgrading),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonInProgress,
			Message: buildRollingUpgradeInProgressMessage(cluster),
		}
	}

	if blueGreenInProgress && cluster.Status.BlueGreen != nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUpgrading),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonInProgress,
			Message: buildBlueGreenUpgradeMessage(cluster),
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionUpgrading),
		Status:  metav1.ConditionFalse,
		Reason:  reasonIdle,
		Message: "No upgrade is currently in progress",
	}
}

func buildBreakGlassConditionMessage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	message := "Break glass mode is active."
	if cluster != nil && cluster.Status.BreakGlass != nil {
		if detail := strings.TrimSpace(cluster.Status.BreakGlass.Message); detail != "" {
			message = ensureSentence(detail)
		}
	}

	return message + " Next step: follow status.breakGlass.steps and set spec.breakGlassAck to status.breakGlass.nonce when it is safe to resume automation."
}

func buildRollingUpgradeInProgressMessage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	from, to := rollingVersionRange(cluster)

	if cluster == nil || cluster.Status.Upgrade == nil {
		return fmt.Sprintf("Rolling upgrade from %s to %s is in progress.", from, to)
	}

	return fmt.Sprintf(
		"Rolling upgrade from %s to %s is in progress (partition=%d).",
		from,
		to,
		cluster.Status.Upgrade.CurrentPartition,
	)
}

func buildRollingUpgradeFailedMessage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	from, to := rollingVersionRange(cluster)
	detail := "The operator recorded a failure."
	if cluster != nil && cluster.Status.Upgrade != nil {
		if message := strings.TrimSpace(cluster.Status.Upgrade.LastErrorMessage); message != "" {
			detail = ensureSentence(message)
		}
	}

	return fmt.Sprintf(
		"Rolling upgrade from %s to %s is paused. %s Next step: set %s to a new non-empty value on this OpenBaoCluster to retry.",
		from,
		to,
		detail,
		upgradeRequestRetryFieldPath,
	)
}

func buildBlueGreenUpgradeMessage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	from, to := blueGreenVersionRange(cluster)
	if cluster == nil || cluster.Status.BlueGreen == nil {
		return fmt.Sprintf("Blue/green upgrade from %s to %s is in progress.", from, to)
	}

	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
		return fmt.Sprintf(
			"Blue/green upgrade from %s to %s is paused in break glass mode. %s",
			from,
			to,
			buildBreakGlassConditionMessage(cluster),
		)
	}

	status := cluster.Status.BlueGreen
	greenRevision := fallbackLabel(status.GreenRevision, "pending")
	blueRevision := fallbackLabel(status.BlueRevision, "current")

	switch status.Phase {
	case openbaov1alpha1.PhaseDeployingGreen:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is deploying Green revision %s.", from, to, greenRevision)
	case openbaov1alpha1.PhaseJoiningMesh:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is joining Green revision %s to the Raft mesh.", from, to, greenRevision)
	case openbaov1alpha1.PhaseSyncing:
		if manualApprovalRequired(cluster) {
			return fmt.Sprintf(
				"Blue/green upgrade from %s to %s is syncing Green revision %s. Manual promotion is required for this upgrade. Next step: set %s to a new non-empty value when you want the operator to promote Green.",
				from,
				to,
				greenRevision,
				upgradeRequestPromoteFieldPath,
			)
		}
		return fmt.Sprintf("Blue/green upgrade from %s to %s is verifying Green revision %s before promotion.", from, to, greenRevision)
	case openbaov1alpha1.PhasePromoting:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is promoting Green revision %s.", from, to, greenRevision)
	case openbaov1alpha1.PhaseDemotingBlue:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is demoting Blue revision %s after promoting Green revision %s.", from, to, blueRevision, greenRevision)
	case openbaov1alpha1.PhaseCleanup:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is cleaning up Blue revision %s after promoting Green revision %s.", from, to, blueRevision, greenRevision)
	case openbaov1alpha1.PhaseRollingBack:
		return fmt.Sprintf(
			"Blue/green upgrade from %s to %s is rolling back to Blue revision %s. %s",
			from,
			to,
			blueRevision,
			rollbackReasonSentence(status.RollbackReason),
		)
	case openbaov1alpha1.PhaseRollbackCleanup:
		return fmt.Sprintf(
			"Blue/green upgrade from %s to %s is finalizing rollback to Blue revision %s. %s",
			from,
			to,
			blueRevision,
			rollbackReasonSentence(status.RollbackReason),
		)
	default:
		return fmt.Sprintf("Blue/green upgrade from %s to %s is in phase %s.", from, to, status.Phase)
	}
}

func rollingVersionRange(cluster *openbaov1alpha1.OpenBaoCluster) (string, string) {
	if cluster == nil || cluster.Status.Upgrade == nil {
		return fallbackLabel("", "unknown"), fallbackLabel("", "unknown")
	}

	return fallbackLabel(cluster.Status.Upgrade.FromVersion, "unknown"), fallbackLabel(cluster.Status.Upgrade.TargetVersion, "unknown")
}

func blueGreenVersionRange(cluster *openbaov1alpha1.OpenBaoCluster) (string, string) {
	if cluster == nil {
		return "unknown", "unknown"
	}

	return fallbackLabel(cluster.Status.CurrentVersion, "unknown"), fallbackLabel(cluster.Spec.Version, "unknown")
}

func manualApprovalRequired(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.ManualPromotionRequired
}

func rollbackReasonSentence(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "Rollback is active."
	}
	return ensureSentence("Rollback reason: " + reason)
}

func fallbackLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func ensureSentence(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	switch {
	case strings.HasSuffix(message, "."),
		strings.HasSuffix(message, "!"),
		strings.HasSuffix(message, "?"):
		return message
	default:
		return message + "."
	}
}

// buildBackupCondition builds the BackingUp condition based on backup job state.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildBackupCondition(backupInProgress bool, backupJobName string) metav1.Condition {
	if backupInProgress {
		message := "Backup in progress"
		if backupJobName != "" {
			message = fmt.Sprintf("Backup Job %s is running", backupJobName)
		}
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionBackingUp),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonInProgress,
			Message: message,
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionBackingUp),
		Status:  metav1.ConditionFalse,
		Reason:  reasonIdle,
		Message: "No backup is currently in progress",
	}
}

// buildStorageConfiguredCondition reports whether the workload is using an explicit
// or consistently resolved storage class, so users can see the effective one-shot choice.
func buildStorageConfiguredCondition(cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState) metav1.Condition {
	desiredStorageClassName := ""
	if cluster.Spec.Storage.StorageClassName != nil {
		desiredStorageClassName = strings.TrimSpace(*cluster.Spec.Storage.StorageClassName)
	}

	if state == nil {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "Storage configuration has not been observed yet",
		}
	}

	if state.DataPVCCount == 0 {
		if desiredStorageClassName != "" {
			return metav1.Condition{
				Type:    string(openbaov1alpha1.ConditionStorageConfigured),
				Status:  metav1.ConditionTrue,
				Reason:  ReasonStorageClassConfigured,
				Message: fmt.Sprintf("Configured to request StorageClass %q when data PVCs are created. This choice becomes effectively immutable after PVC creation.", desiredStorageClassName),
			}
		}
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonStorageClassPending,
			Message: "No data PVCs are present yet and spec.storage.storageClassName is unset. The cluster will rely on the default StorageClass when PVCs are created; set it explicitly on new clusters if you need a specific class.",
		}
	}

	if state.DataPVCStorageClassUnset && len(state.DataPVCStorageClassNames) == 0 {
		if desiredStorageClassName != "" {
			return metav1.Condition{
				Type:    string(openbaov1alpha1.ConditionStorageConfigured),
				Status:  metav1.ConditionFalse,
				Reason:  ReasonStorageClassMismatch,
				Message: fmt.Sprintf("spec.storage.storageClassName=%q does not match the observed data PVCs, which were created without a StorageClass. Storage class selection is effectively immutable after PVC creation.", desiredStorageClassName),
			}
		}
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonStorageClassUnset,
			Message: fmt.Sprintf("All %d data PVCs were created without a StorageClass. Set spec.storage.storageClassName explicitly on new clusters if you need a specific class; the effective storage path is immutable after PVC creation.", state.DataPVCCount),
		}
	}

	if state.DataPVCStorageClassUnset || len(state.DataPVCStorageClassNames) > 1 {
		observed := append([]string{}, state.DataPVCStorageClassNames...)
		if state.DataPVCStorageClassUnset {
			observed = append(observed, "<unset>")
		}
		sort.Strings(observed)
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonStorageClassInconsistent,
			Message: fmt.Sprintf("Observed inconsistent StorageClass values across %d data PVCs: %s. All OpenBao data PVCs should use one effective storage class.", state.DataPVCCount, strings.Join(observed, ", ")),
		}
	}

	observedStorageClassName := state.DataPVCStorageClassNames[0]
	if desiredStorageClassName == "" {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonStorageClassDefaulted,
			Message: fmt.Sprintf("Using default StorageClass %q on %d data PVCs. Set spec.storage.storageClassName explicitly on new clusters if you need a specific class; this choice is effectively immutable after PVC creation.", observedStorageClassName, state.DataPVCCount),
		}
	}
	if desiredStorageClassName != observedStorageClassName {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionStorageConfigured),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonStorageClassMismatch,
			Message: fmt.Sprintf("spec.storage.storageClassName=%q does not match the observed data PVC StorageClass %q. Storage class selection is effectively immutable after PVC creation.", desiredStorageClassName, observedStorageClassName),
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionStorageConfigured),
		Status:  metav1.ConditionTrue,
		Reason:  ReasonStorageClassConfigured,
		Message: fmt.Sprintf("Using configured StorageClass %q on %d data PVCs. This choice is effectively immutable after PVC creation.", observedStorageClassName, state.DataPVCCount),
	}
}

// buildInitializedCondition builds the OpenBaoInitialized condition from pod labels.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildInitializedCondition(initialized, present bool) metav1.Condition {
	if !present {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoInitialized),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "OpenBao initialization state is not yet available via service registration",
		}
	}

	if initialized {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoInitialized),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonInitialized,
			Message: "OpenBao reports initialized",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionOpenBaoInitialized),
		Status:  metav1.ConditionFalse,
		Reason:  ReasonNotInitialized,
		Message: "OpenBao reports not initialized",
	}
}

// buildSealedCondition builds the OpenBaoSealed condition from pod labels.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildSealedCondition(sealed, present bool) metav1.Condition {
	if !present {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoSealed),
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: "OpenBao seal state is not yet available via service registration",
		}
	}

	if sealed {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoSealed),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonSealed,
			Message: "OpenBao reports sealed",
		}
	}

	return metav1.Condition{
		Type:    string(openbaov1alpha1.ConditionOpenBaoSealed),
		Status:  metav1.ConditionFalse,
		Reason:  ReasonUnsealed,
		Message: "OpenBao reports unsealed",
	}
}

// buildLeaderCondition builds the OpenBaoLeader condition from leader count.
// ObservedGeneration and LastTransitionTime must be set by the caller.
func buildLeaderCondition(leaderCount int, leaderName string) metav1.Condition {
	switch leaderCount {
	case 0:
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonLeaderUnknown,
			Message: "No active leader label observed on pods",
		}
	case 1:
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:  metav1.ConditionTrue,
			Reason:  ReasonLeaderFound,
			Message: fmt.Sprintf("Leader is %s", leaderName),
		}
	default:
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonMultipleLeaders,
			Message: fmt.Sprintf("Multiple leaders detected via pod labels (%d)", leaderCount),
		}
	}
}

// applyAllConditions computes and sets all status conditions from cluster state.
// This consolidates condition logic to eliminate duplicate code paths.
func applyAllConditions(
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *clusterState,
	admissionStatus *admission.Status,
	now metav1.Time,
) {
	gen := cluster.Generation

	// OpenBao initialized condition (from pod0 labels)
	initCond := buildInitializedCondition(state.Initialized, state.InitializedKnown)
	initCond.ObservedGeneration = gen
	initCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, initCond)

	// OpenBao sealed condition (from pod0 labels)
	sealedCond := buildSealedCondition(state.Sealed, state.SealedKnown)
	sealedCond.ObservedGeneration = gen
	sealedCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, sealedCond)

	// Leader condition
	leaderCond := buildLeaderCondition(state.LeaderCount, state.LeaderName)
	leaderCond.ObservedGeneration = gen
	leaderCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, leaderCond)

	// Available condition
	availableCond := buildAvailableCondition(cluster, state.ReadyReplicas)
	availableCond.ObservedGeneration = gen
	availableCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, availableCond)

	// Degraded condition
	degradedCond := buildDegradedCondition(cluster, state.UpgradeFailed)
	degradedCond.ObservedGeneration = gen
	degradedCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, degradedCond)

	// Upgrading condition
	upgradingCond := buildUpgradingCondition(cluster)
	upgradingCond.ObservedGeneration = gen
	upgradingCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, upgradingCond)

	// Backup condition
	backupCond := buildBackupCondition(state.BackupInProgress, state.BackupJobName)
	backupCond.ObservedGeneration = gen
	backupCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, backupCond)

	// Self-init user access bootstrap condition
	userAccessCond := buildUserAccessBootstrapCondition(cluster)
	userAccessCond.ObservedGeneration = gen
	userAccessCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, userAccessCond)

	// Storage configuration condition
	storageCond := buildStorageConfiguredCondition(cluster, state)
	storageCond.ObservedGeneration = gen
	storageCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, storageCond)

	// Etcd encryption warning (always set)
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionEtcdEncryptionWarning),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gen,
		LastTransitionTime: now,
		Reason:             ReasonEtcdEncryptionUnknown,
		Message:            "The operator cannot verify etcd encryption status. Ensure etcd encryption at rest is enabled in your Kubernetes cluster to protect Secrets (including unseal keys and root tokens) stored in etcd.",
	})

	// Security risk condition for Development profile
	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionSecurityRisk),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gen,
			LastTransitionTime: now,
			Reason:             ReasonDevelopmentProfile,
			Message:            "Cluster is using Development profile with relaxed security. Not suitable for production.",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionSecurityRisk))
	}

	// Production ready condition
	admissionReady := admissionStatus == nil || admissionStatus.OverallReady
	admissionSummary := ""
	if admissionStatus != nil {
		admissionSummary = admissionStatus.SummaryMessage()
	}
	productionStatus, productionReason, productionMessage := evaluateProductionReady(cluster, admissionReady, admissionSummary)
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionProductionReady),
		Status:             productionStatus,
		ObservedGeneration: gen,
		LastTransitionTime: now,
		Reason:             productionReason,
		Message:            productionMessage,
	})

	applyNodeSecurityCapabilityMismatchCondition(cluster, state, gen, now)
}

func applyNodeSecurityCapabilityMismatchCondition(cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState, gen int64, now metav1.Time) {
	appArmorEnabled := cluster.Spec.WorkloadHardening != nil && cluster.Spec.WorkloadHardening.AppArmorEnabled
	if !appArmorEnabled {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch))
		return
	}

	cond := metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: gen,
		LastTransitionTime: now,
		Reason:             reasonReady,
		Message:            "No node security capability mismatch detected for enabled workload hardening settings",
	}

	if state != nil && state.StatefulSet != nil {
		for _, ssCond := range state.StatefulSet.Status.Conditions {
			if ssCond.Type != "ReplicaFailure" {
				continue
			}
			msg := strings.ToLower(ssCond.Message)
			if strings.Contains(msg, "apparmor") {
				cond.Status = metav1.ConditionTrue
				cond.Reason = ReasonAppArmorUnsupported
				cond.Message = "AppArmor is enabled (spec.workloadHardening.appArmorEnabled=true) but the workload cannot be admitted/scheduled due to AppArmor support mismatch: " + ssCond.Message
				break
			}
		}
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, cond)
}

// computePhase determines the cluster phase from state.
func computePhase(state *clusterState) openbaov1alpha1.ClusterPhase {
	if state.UpgradeFailed {
		return openbaov1alpha1.ClusterPhaseFailed
	}
	if state.UpgradeInProgress {
		return openbaov1alpha1.ClusterPhaseUpgrading
	}
	if state.BackupInProgress {
		return openbaov1alpha1.ClusterPhaseBackingUp
	}
	if state.Available {
		return openbaov1alpha1.ClusterPhaseRunning
	}
	return openbaov1alpha1.ClusterPhaseInitializing
}
