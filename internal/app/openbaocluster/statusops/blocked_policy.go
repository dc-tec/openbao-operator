package statusops

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// BlockedPolicyInput contains the state used to project a blocked
// reconciliation decision into cluster status.
type BlockedPolicyInput struct {
	Cluster                       *openbaov1alpha1.OpenBaoCluster
	CloudUnsealIdentityApplicable bool
	Now                           metav1.Time
}

// ApplyPausedPolicy marks status evaluations that cannot run while
// reconciliation is paused.
func ApplyPausedPolicy(input BlockedPolicyInput) {
	cluster := input.Cluster
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	setBlockedCondition(cluster, openbaov1alpha1.ConditionAvailable, metav1.ConditionUnknown, reasonPaused, "Reconciliation is paused; availability is not being evaluated")
	setBlockedCondition(cluster, openbaov1alpha1.ConditionDegraded, metav1.ConditionFalse, reasonPaused, "Cluster is paused; no new degradation has been evaluated")
	ApplyTLSReadyCondition(cluster, ConditionResult{
		Status:  metav1.ConditionUnknown,
		Reason:  reasonPaused,
		Message: "TLS readiness is not being evaluated while reconciliation is paused",
	})
	ApplyAPIServerNetworkReadyCondition(cluster, ConditionResult{
		Status:  metav1.ConditionUnknown,
		Reason:  reasonPaused,
		Message: "Kubernetes API egress readiness is not being evaluated while reconciliation is paused",
	})

	if portopenbao.UsesACMEMode(cluster) {
		ApplyACMEIntegrationReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "ACME integration prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMEIntegrationReady))
	}

	if portopenbao.HasAuditFileStorage(cluster) {
		ApplyAuditFileStorageReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "Audit file storage readiness is not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionAuditFileStorageReady))
	}

	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		ApplyGatewayIntegrationReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "Gateway integration prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionGatewayIntegrationReady))
	}

	if cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled {
		ApplyIngressIntegrationReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "Ingress integration prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionIngressIntegrationReady))
	}

	if cluster.Spec.Backup != nil {
		ApplyBackupConfigurationReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "Backup Job prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionBackupConfigurationReady))
	}

	if input.CloudUnsealIdentityApplicable {
		ApplyCloudUnsealIdentityReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "Cloud KMS unseal identity prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionCloudUnsealIdentityReady))
	}

	ApplyUserAccessBootstrapCondition(cluster, input.Now)
}

// ApplyProfileNotSetPolicy marks status evaluations that cannot run until the
// cluster profile is set.
func ApplyProfileNotSetPolicy(input BlockedPolicyInput) {
	cluster := input.Cluster
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	setBlockedCondition(cluster, openbaov1alpha1.ConditionAvailable, metav1.ConditionFalse, ReasonProfileNotSet, "spec.profile must be explicitly set to Hardened or Development; reconciliation is blocked until set")
	setBlockedCondition(cluster, openbaov1alpha1.ConditionDegraded, metav1.ConditionTrue, ReasonProfileNotSet, "spec.profile is not set; defaults may be inappropriate for production and could lead to insecure deployment")
	ApplyTLSReadyCondition(cluster, ConditionResult{
		Status:  metav1.ConditionUnknown,
		Reason:  ReasonProfileNotSet,
		Message: "TLS readiness is not being evaluated until spec.profile is set",
	})
	ApplyAPIServerNetworkReadyCondition(cluster, ConditionResult{
		Status:  metav1.ConditionUnknown,
		Reason:  ReasonProfileNotSet,
		Message: "Kubernetes API egress readiness is not being evaluated until spec.profile is set",
	})

	if portopenbao.UsesACMEMode(cluster) {
		ApplyACMEIntegrationReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "ACME integration prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMEIntegrationReady))
	}

	if portopenbao.HasAuditFileStorage(cluster) {
		ApplyAuditFileStorageReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "Audit file storage readiness is not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionAuditFileStorageReady))
	}

	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		ApplyGatewayIntegrationReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "Gateway integration prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionGatewayIntegrationReady))
	}

	if cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled {
		ApplyIngressIntegrationReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "Ingress integration prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionIngressIntegrationReady))
	}

	if cluster.Spec.Backup != nil {
		ApplyBackupConfigurationReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "Backup Job prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionBackupConfigurationReady))
	}

	if input.CloudUnsealIdentityApplicable {
		ApplyCloudUnsealIdentityReadyCondition(cluster, ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "Cloud KMS unseal identity prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionCloudUnsealIdentityReady))
	}

	setBlockedCondition(cluster, openbaov1alpha1.ConditionProductionReady, metav1.ConditionFalse, ReasonProfileNotSet, "Cluster cannot be considered production-ready until spec.profile is explicitly set")
	ApplyUserAccessBootstrapCondition(cluster, input.Now)
}

func setBlockedCondition(
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	status metav1.ConditionStatus,
	reason string,
	message string,
) {
	setClusterConditionResult(cluster, conditionType, ConditionResult{
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}
