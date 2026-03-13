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

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             reasonPaused,
		Message:            "Reconciliation is paused; availability is not being evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             reasonPaused,
		Message:            "Cluster is paused; no new degradation has been evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             reasonPaused,
		Message:            "TLS readiness is not being evaluated while reconciliation is paused",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAPIServerNetworkReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             reasonPaused,
		Message:            "Kubernetes API egress readiness is not being evaluated while reconciliation is paused",
	})

	if portopenbao.UsesACMEMode(cluster) {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionACMEIntegrationReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             reasonPaused,
			Message:            "ACME integration prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMEIntegrationReady))
	}

	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionGatewayIntegrationReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             reasonPaused,
			Message:            "Gateway integration prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionGatewayIntegrationReady))
	}

	if cluster.Spec.Backup != nil {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionBackupConfigurationReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             reasonPaused,
			Message:            "Backup Job prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionBackupConfigurationReady))
	}

	if _, applicable := appopenbaocluster.DescribeCloudUnsealIdentity(cluster); applicable {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionCloudUnsealIdentityReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             reasonPaused,
			Message:            "Cloud KMS unseal identity prerequisites are not being evaluated while reconciliation is paused",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionCloudUnsealIdentityReady))
	}

	userAccessCond := buildUserAccessBootstrapCondition(cluster)
	userAccessCond.ObservedGeneration = cluster.Generation
	userAccessCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, userAccessCond)

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

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "spec.profile must be explicitly set to Hardened or Development; reconciliation is blocked until set",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "spec.profile is not set; defaults may be inappropriate for production and could lead to insecure deployment",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "TLS readiness is not being evaluated until spec.profile is set",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAPIServerNetworkReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "Kubernetes API egress readiness is not being evaluated until spec.profile is set",
	})

	if portopenbao.UsesACMEMode(cluster) {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionACMEIntegrationReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonProfileNotSet,
			Message:            "ACME integration prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMEIntegrationReady))
	}

	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionGatewayIntegrationReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonProfileNotSet,
			Message:            "Gateway integration prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionGatewayIntegrationReady))
	}

	if cluster.Spec.Backup != nil {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionBackupConfigurationReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonProfileNotSet,
			Message:            "Backup Job prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionBackupConfigurationReady))
	}

	if _, applicable := appopenbaocluster.DescribeCloudUnsealIdentity(cluster); applicable {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionCloudUnsealIdentityReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonProfileNotSet,
			Message:            "Cloud KMS unseal identity prerequisites are not being evaluated until spec.profile is set",
		})
	} else {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionCloudUnsealIdentityReady))
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionProductionReady),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "Cluster cannot be considered production-ready until spec.profile is explicitly set",
	})

	userAccessCond := buildUserAccessBootstrapCondition(cluster)
	userAccessCond.ObservedGeneration = cluster.Generation
	userAccessCond.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, userAccessCond)

	if err := r.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for missing profile on OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster missing profile")
	return nil
}
