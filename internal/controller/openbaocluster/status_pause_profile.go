package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func (r *OpenBaoClusterReconciler) updateStatusForPaused(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	now := metav1.Now()

	setPausedCondition(cluster, openbaov1alpha1.ConditionAvailable, metav1.ConditionUnknown, "Reconciliation is paused; availability is not being evaluated")
	setPausedCondition(cluster, openbaov1alpha1.ConditionDegraded, metav1.ConditionFalse, "Cluster is paused; no new degradation has been evaluated")
	setTLSReadyEvaluatedCondition(cluster, statusConditionResult{
		Status:  metav1.ConditionUnknown,
		Reason:  reasonPaused,
		Message: "TLS readiness is not being evaluated while reconciliation is paused",
	})
	setAPIServerNetworkReadyEvaluatedCondition(cluster, appopenbaocluster.APIServerNetworkResult{
		Status:  metav1.ConditionUnknown,
		Reason:  reasonPaused,
		Message: "Kubernetes API egress readiness is not being evaluated while reconciliation is paused",
	})

	if portopenbao.UsesACMEMode(cluster) {
		setACMEIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.ACMEIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "ACME integration prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMEIntegrationReady))
	}

	if portopenbao.HasAuditFileStorage(cluster) {
		setAuditFileStorageReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "Audit file storage readiness is not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionAuditFileStorageReady))
	}

	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		setGatewayIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "Gateway integration prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionGatewayIntegrationReady))
	}

	if cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled {
		setIngressIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.IngressIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "Ingress integration prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionIngressIntegrationReady))
	}

	if cluster.Spec.Backup != nil {
		setBackupConfigurationReadyEvaluatedCondition(cluster, appopenbaocluster.BackupConfigurationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "Backup Job prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionBackupConfigurationReady))
	}

	if _, applicable := appopenbaocluster.DescribeCloudUnsealIdentity(cluster); applicable {
		setCloudUnsealIdentityReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonPaused,
			Message: "Cloud KMS unseal identity prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionCloudUnsealIdentityReady))
	}

	appopenbaocluster.ApplyUserAccessBootstrapCondition(cluster, now)

	if err := r.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for paused OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for paused OpenBaoCluster")
	return nil
}

func (r *OpenBaoClusterReconciler) updateStatusForProfileNotSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	now := metav1.Now()
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	setProfileNotSetCondition(cluster, openbaov1alpha1.ConditionAvailable, metav1.ConditionFalse, "spec.profile must be explicitly set to Hardened or Development; reconciliation is blocked until set")
	setProfileNotSetCondition(cluster, openbaov1alpha1.ConditionDegraded, metav1.ConditionTrue, "spec.profile is not set; defaults may be inappropriate for production and could lead to insecure deployment")
	setTLSReadyEvaluatedCondition(cluster, statusConditionResult{
		Status:  metav1.ConditionUnknown,
		Reason:  ReasonProfileNotSet,
		Message: "TLS readiness is not being evaluated until spec.profile is set",
	})
	setAPIServerNetworkReadyEvaluatedCondition(cluster, appopenbaocluster.APIServerNetworkResult{
		Status:  metav1.ConditionUnknown,
		Reason:  ReasonProfileNotSet,
		Message: "Kubernetes API egress readiness is not being evaluated until spec.profile is set",
	})

	if portopenbao.UsesACMEMode(cluster) {
		setACMEIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.ACMEIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "ACME integration prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMEIntegrationReady))
	}

	if portopenbao.HasAuditFileStorage(cluster) {
		setAuditFileStorageReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "Audit file storage readiness is not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionAuditFileStorageReady))
	}

	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		setGatewayIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.GatewayIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "Gateway integration prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionGatewayIntegrationReady))
	}

	if cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled {
		setIngressIntegrationReadyEvaluatedCondition(cluster, appopenbaocluster.IngressIntegrationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "Ingress integration prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionIngressIntegrationReady))
	}

	if cluster.Spec.Backup != nil {
		setBackupConfigurationReadyEvaluatedCondition(cluster, appopenbaocluster.BackupConfigurationResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "Backup Job prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionBackupConfigurationReady))
	}

	if _, applicable := appopenbaocluster.DescribeCloudUnsealIdentity(cluster); applicable {
		setCloudUnsealIdentityReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonProfileNotSet,
			Message: "Cloud KMS unseal identity prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionCloudUnsealIdentityReady))
	}

	setProfileNotSetCondition(cluster, openbaov1alpha1.ConditionProductionReady, metav1.ConditionFalse, "Cluster cannot be considered production-ready until spec.profile is explicitly set")

	appopenbaocluster.ApplyUserAccessBootstrapCondition(cluster, now)

	if err := r.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for missing profile on OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster missing profile")
	return nil
}
